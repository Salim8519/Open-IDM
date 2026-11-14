// Package ipc is how local clients (the CLI and the browser extension's native messaging
// host) find and call a running Peregrine app. It uses only the standard library, so the
// small programs that import it do not pull in the engine.
package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RPCError is returned to clients as {"error": {"code","message"}}.
type RPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

// IPCInfo is written to the user's config dir so the native messaging host and CLI
// can find the running app.
type IPCInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// ConfigDir returns Peregrine's per-user config/data directory.
func ConfigDir() (string, error) {
	if d := os.Getenv("PEREGRINE_HOME"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Peregrine"), nil
}

func ipcPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "ipc.json"), nil
}

// WriteIPCInfo publishes the server address for local clients (file mode 0600).
func WriteIPCInfo(port int, token string) error {
	p, err := ipcPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(IPCInfo{Port: port, Token: token, PID: os.Getpid()})
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// RemoveIPCInfo deletes the discovery file (on clean shutdown).
func RemoveIPCInfo() {
	if p, err := ipcPath(); err == nil {
		_ = os.Remove(p)
	}
}

// ReadIPCInfo loads the discovery file.
func ReadIPCInfo() (IPCInfo, error) {
	var info IPCInfo
	p, err := ipcPath()
	if err != nil {
		return info, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(b, &info)
	return info, err
}

// Client calls a running app's RPC server.
type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

// Dial returns a client for the running app, or an error if it isn't running.
func Dial() (*Client, error) {
	info, err := ReadIPCInfo()
	if err != nil {
		return nil, fmt.Errorf("the Peregrine app is not running: %w", err)
	}
	c := &Client{Base: fmt.Sprintf("http://127.0.0.1:%d", info.Port), Token: info.Token, HTTP: &http.Client{Timeout: 30 * time.Second}}
	if _, err := c.Call("version", nil); err != nil {
		return nil, fmt.Errorf("the Peregrine app is not responding: %w", err)
	}
	return c, nil
}

// Call invokes method and returns the raw JSON result.
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{"method": method, "params": params})
	req, _ := http.NewRequest(http.MethodPost, c.Base+"/rpc", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Result, nil
}

// ErrNotRunning is returned by helpers when no app instance is reachable.
var ErrNotRunning = errors.New("the Peregrine app is not running")
