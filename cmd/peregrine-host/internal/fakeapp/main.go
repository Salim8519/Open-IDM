// Command fakeapp is a stand-in for the Peregrine desktop app used by the extension's
// end-to-end tests. It serves the same /rpc API (127.0.0.1 + bearer token), publishes
// ipc.json into PEREGRINE_HOME, and appends every call it receives to a JSON-lines file.
//
//	PEREGRINE_HOME=<tmp> fakeapp -record <tmp>/calls.jsonl
//
// It prints "READY <port>" on stdout once listening and exits when stdin closes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/Salim8519/Open-IDM/internal/api"
)

func main() {
	record := flag.String("record", "", "append received calls to this JSON-lines file")
	minSize := flag.Int64("min-size", 1<<20, "minExtSize reported by getSettings")
	flag.Parse()
	if os.Getenv("PEREGRINE_HOME") == "" {
		fmt.Fprintln(os.Stderr, "fakeapp: PEREGRINE_HOME must be set (refusing to touch the real config dir)")
		os.Exit(2)
	}

	var mu sync.Mutex
	token := api.NewToken()
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if *record != "" && req.Method != "version" {
			mu.Lock()
			if f, err := os.OpenFile(*record, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
				b, _ := json.Marshal(map[string]any{"method": req.Method, "params": req.Params})
				_, _ = f.Write(append(b, '\n'))
				_ = f.Close()
			}
			mu.Unlock()
		}
		var result any
		switch req.Method {
		case "version":
			result = map[string]string{"version": "9.9.9-fake"}
		case "getSettings":
			result = map[string]any{"minExtSize": *minSize, "autoStartExt": false, "language": "en", "downloadDir": "C:\\secret", "proxy": "http://user:pw@proxy"}
		case "capture":
			result = map[string]any{"queued": false, "prompted": true}
		case "add":
			result = map[string]any{"id": "fake-1", "status": "queued"}
		case "list":
			result = []map[string]any{{"id": "fake-1", "url": "https://example.com/x?token=1", "filename": "x.zip", "status": "queued"}}
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "unknown_method", "message": "unknown method"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	})

	srv, err := listen(mux)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeapp:", err)
		os.Exit(1)
	}
	if err := api.WriteIPCInfo(srv.port, token); err != nil {
		fmt.Fprintln(os.Stderr, "fakeapp:", err)
		os.Exit(1)
	}
	fmt.Printf("READY %d\n", srv.port)
	_, _ = io.Copy(io.Discard, os.Stdin) // run until the test closes stdin
	api.RemoveIPCInfo()
}
