package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Server serves the RPC API on 127.0.0.1 with a bearer token.
//
//	POST /rpc            {"method": "...", "params": {...}} -> {"result": ...} | {"error": {...}}
//	GET  /events?token=  Server-Sent Events stream of Event JSON
//	GET  /*              optional static UI (browser mode / e2e)
type Server struct {
	svc    *Service
	token  string
	ln     net.Listener
	srv    *http.Server
	static fs.FS
}

// NewToken returns a random 32-byte hex token.
func NewToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Listen starts the server on 127.0.0.1:port (0 = random). static may be nil.
func Listen(svc *Service, port int, token string, static fs.FS) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	s := &Server{svc: svc, token: token, ln: ln, static: static}
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.rpc)
	mux.HandleFunc("/events", s.events)
	if static != nil {
		mux.Handle("/", http.FileServerFS(static))
	}
	s.srv = &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

// Port returns the listening port.
func (s *Server) Port() int { return s.ln.Addr().(*net.TCPAddr).Port }

// Token returns the auth token.
func (s *Server) Token() string { return s.token }

// Close stops the server.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

// guard blocks DNS-rebinding (Host must be loopback) and cross-site requests.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r.Host) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin, host string) bool {
	return origin == "http://"+host
}

func (s *Server) authorized(r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) == 1
}

type rpcRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcResponse struct {
	Result any       `json:"result,omitempty"`
	Error  *RPCError `json:"error,omitempty"`
}

func (s *Server) rpc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, rpcResponse{Error: &RPCError{Code: "bad_request", Message: err.Error()}})
		return
	}
	res, err := s.svc.Call(r.Context(), req.Method, req.Params)
	if err != nil {
		writeJSON(w, http.StatusOK, rpcResponse{Error: toRPCError(err)})
		return
	}
	writeJSON(w, http.StatusOK, rpcResponse{Result: res})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	ch := make(chan Event, 256)
	unsub := s.svc.Subscribe(func(e Event) {
		select {
		case ch <- e:
		default: // slow client: drop; progress events are periodic anyway
		}
	})
	defer unsub()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

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
