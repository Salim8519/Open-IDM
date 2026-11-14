package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Salim8519/Peregrine/internal/ipc"
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

// The discovery file and the client live in internal/ipc (standard library only), so the
// native messaging host stays small. These names keep existing callers working.
type (
	IPCInfo = ipc.IPCInfo
	Client  = ipc.Client
)

// ConfigDir returns Peregrine's per-user config/data directory.
func ConfigDir() (string, error) { return ipc.ConfigDir() }

// WriteIPCInfo publishes the server address for local clients (file mode 0600).
func WriteIPCInfo(port int, token string) error { return ipc.WriteIPCInfo(port, token) }

// RemoveIPCInfo deletes the discovery file (on clean shutdown).
func RemoveIPCInfo() { ipc.RemoveIPCInfo() }

// ReadIPCInfo loads the discovery file.
func ReadIPCInfo() (IPCInfo, error) { return ipc.ReadIPCInfo() }

// Dial returns a client for the running app, or an error if it isn't running.
func Dial() (*Client, error) { return ipc.Dial() }

// ErrNotRunning is returned by helpers when no app instance is reachable.
var ErrNotRunning = ipc.ErrNotRunning
