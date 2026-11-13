package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/api"
)

// hostVersion is the native messaging host version (set with -ldflags at release time).
var hostVersion = "0.1.0"

// caller is the part of api.Client the host uses (an interface so tests can fake it).
type caller interface {
	Call(method string, params any) (json.RawMessage, error)
}

// errAppMissing means there is no app executable to launch.
var errAppMissing = errors.New("app executable not found")

type host struct {
	log        *log.Logger
	dial       func() (caller, error)
	launch     func() error
	launchWait time.Duration
	pollEvery  time.Duration
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type response struct {
	ID     json.RawMessage `json:"id"`
	Result any             `json:"result,omitempty"`
	Error  *api.RPCError   `json:"error,omitempty"`
}

// Error codes returned to the extension (in addition to the app's own codes).
const (
	codeBadRequest     = "bad_request"
	codeBadParams      = "bad_params"
	codeForbidden      = "method_not_allowed"
	codeAppUnavailable = "app_unavailable"
	codeTooLarge       = "too_large"
)

func rpcErr(code, format string, a ...any) *api.RPCError {
	return &api.RPCError{Code: code, Message: fmt.Sprintf(format, a...)}
}

// allowedMethods is the complete list of methods the extension may call. Everything else
// (remove, updateSettings, openFile, ...) is refused even though the app supports it.
var allowedMethods = map[string]bool{
	"ping":        true,
	"version":     true,
	"capture":     true,
	"add":         true,
	"getSettings": true,
	"list":        true,
}

// Methods that start the app when it is not running. getSettings/list/ping never launch
// it: the extension calls them in the background and must not pop the app up.
var launchingMethods = map[string]bool{"version": true, "capture": true, "add": true}

// serve runs the message loop until EOF or a framing error.
func (h *host) serve(in io.Reader, out io.Writer) error {
	for {
		msg, err := readFrame(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			// Framing is lost (oversized or truncated frame); we cannot resync.
			h.log.Printf("read: %v", err)
			_ = h.reply(out, response{ID: json.RawMessage("null"), Error: rpcErr(codeBadRequest, "%v", err)})
			return err
		}
		if err := h.reply(out, h.handle(msg)); err != nil {
			return err
		}
	}
}

func (h *host) reply(out io.Writer, r response) error {
	if len(r.ID) == 0 {
		r.ID = json.RawMessage("null")
	}
	b, err := json.Marshal(r)
	if err == nil && len(b) > maxOutMessage {
		err = errTooLarge
	}
	if err != nil {
		b, _ = json.Marshal(response{ID: r.ID, Error: rpcErr(codeTooLarge, "reply too large")})
	}
	return writeFrame(out, b)
}

// handle processes one decoded frame and never panics on bad input.
func (h *host) handle(msg []byte) response {
	var req request
	if err := json.Unmarshal(msg, &req); err != nil {
		return response{Error: rpcErr(codeBadRequest, "invalid JSON")}
	}
	if !validID(req.ID) {
		return response{Error: rpcErr(codeBadRequest, "id must be a number or string")}
	}
	resp := response{ID: req.ID}
	if !allowedMethods[req.Method] {
		h.log.Printf("refused method %q", truncate(req.Method, 40))
		resp.Error = rpcErr(codeForbidden, "method not allowed")
		return resp
	}
	start := time.Now()
	res, rerr := h.dispatch(req.Method, req.Params)
	if rerr != nil {
		resp.Error = rerr
		h.log.Printf("%s -> error %s: %s (%v)", req.Method, rerr.Code, redactText(rerr.Message), time.Since(start).Round(time.Millisecond))
	} else {
		resp.Result = res
		h.log.Printf("%s -> ok (%v)", req.Method, time.Since(start).Round(time.Millisecond))
	}
	return resp
}

func validID(id json.RawMessage) bool {
	if len(id) == 0 {
		return true // treated as null
	}
	var v any
	if json.Unmarshal(id, &v) != nil {
		return false
	}
	switch v.(type) {
	case float64, string, nil:
		return true
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func (h *host) dispatch(method string, params json.RawMessage) (any, *api.RPCError) {
	if method == "ping" {
		out := map[string]any{"host": hostVersion, "app": false}
		if c, err := h.dial(); err == nil {
			out["app"] = true
			if raw, err := c.Call("version", nil); err == nil {
				var v struct {
					Version string `json:"version"`
				}
				if json.Unmarshal(raw, &v) == nil {
					out["appVersion"] = v.Version
				}
			}
			h.hello(c)
		}
		return out, nil
	}

	var fwd any
	switch method {
	case "capture":
		c, err := sanitizeCapture(params)
		if err != nil {
			return nil, rpcErr(codeBadParams, "%v", err)
		}
		h.log.Printf("capture %s", redactURL(c.URL))
		fwd = c
	case "add":
		r, err := sanitizeRequest(params)
		if err != nil {
			return nil, rpcErr(codeBadParams, "%v", err)
		}
		h.log.Printf("add %s", redactURL(r.URL))
		fwd = r
	}

	c, rerr := h.app(launchingMethods[method])
	if rerr != nil {
		return nil, rerr
	}
	if launchingMethods[method] {
		allowForeground() // the app may show its Add dialog in front of the browser
	}
	raw, err := c.Call(method, fwd)
	if err != nil {
		var re *api.RPCError
		if errors.As(err, &re) {
			return nil, re
		}
		return nil, rpcErr(codeAppUnavailable, "Peregrine did not answer: %s", redactText(err.Error()))
	}
	switch method {
	case "getSettings":
		h.hello(c) // sent on extension install and browser start
		return filterSettings(raw), nil
	case "list":
		return filterList(raw), nil
	}
	return raw, nil
}

// hello tells the app that its extension is installed and working, which closes the
// app's "Connect your browser" prompt. Best effort: an old app without the method is fine.
func (h *host) hello(c caller) { _, _ = c.Call("extensionHello", nil) }

// app returns a client for the running app, starting it first when allowed.
func (h *host) app(launch bool) (caller, *api.RPCError) {
	c, err := h.dial()
	if err == nil {
		return c, nil
	}
	if !launch {
		return nil, rpcErr(codeAppUnavailable, "Peregrine is not running")
	}
	h.log.Printf("app not reachable (%s); launching", redactText(err.Error()))
	if err := h.launch(); err != nil {
		h.log.Printf("launch: %v", err)
		return nil, rpcErr(codeAppUnavailable, "could not start Peregrine")
	}
	deadline := time.Now().Add(h.launchWait)
	for time.Now().Before(deadline) {
		time.Sleep(h.pollEvery)
		if c, err := h.dial(); err == nil {
			return c, nil
		}
	}
	return nil, rpcErr(codeAppUnavailable, "Peregrine did not start in time")
}

// allowedHeaders are the only request headers the extension may forward.
var allowedHeaders = map[string]string{
	"cookie":          "Cookie",
	"referer":         "Referer",
	"user-agent":      "User-Agent",
	"accept-language": "Accept-Language",
}

const (
	maxURLLen    = 16 << 10
	maxHeaderLen = 64 << 10
)

func checkURL(raw string) error {
	if raw == "" {
		return errors.New("url is required")
	}
	if len(raw) > maxURLLen {
		return errors.New("url too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid url")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ftp":
	default:
		return fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("url has no host")
	}
	return nil
}

func cleanHeaders(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		name, ok := allowedHeaders[strings.ToLower(strings.TrimSpace(k))]
		if !ok || v == "" || len(v) > maxHeaderLen || strings.ContainsAny(v, "\r\n\x00") {
			continue
		}
		out[name] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanPageURL(raw string) string {
	if raw == "" || len(raw) > maxURLLen {
		return ""
	}
	if u, err := url.Parse(raw); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return raw
}

// sanitizeCapture keeps only fields the extension is allowed to set. The download folder
// is never taken from the browser; the app (or the user in the add dialog) chooses it.
func sanitizeCapture(params json.RawMessage) (api.Capture, error) {
	var in api.Capture
	if len(params) == 0 || json.Unmarshal(params, &in) != nil {
		return api.Capture{}, errors.New("params must be an object")
	}
	if err := checkURL(in.URL); err != nil {
		return api.Capture{}, err
	}
	out := api.Capture{
		Request: engine.Request{
			URL:      in.URL,
			Filename: truncate(in.Filename, 255),
			Headers:  cleanHeaders(in.Headers),
		},
		Source:   "extension",
		PageURL:  cleanPageURL(in.PageURL),
		MimeType: truncate(in.MimeType, 255),
	}
	if in.Size > 0 {
		out.Size = in.Size
	}
	return out, nil
}

func sanitizeRequest(params json.RawMessage) (engine.Request, error) {
	var in engine.Request
	if len(params) == 0 || json.Unmarshal(params, &in) != nil {
		return engine.Request{}, errors.New("params must be an object")
	}
	if err := checkURL(in.URL); err != nil {
		return engine.Request{}, err
	}
	return engine.Request{
		URL:      in.URL,
		Filename: truncate(in.Filename, 255),
		Headers:  cleanHeaders(in.Headers),
		Paused:   in.Paused,
	}, nil
}

// filterSettings returns only what the extension needs; download folders, proxy URLs
// (which may embed credentials) and the like never leave the app.
func filterSettings(raw json.RawMessage) any {
	var s engine.Settings
	_ = json.Unmarshal(raw, &s)
	return map[string]any{
		"minExtSize":   s.MinExtSize,
		"autoStartExt": s.AutoStartExt,
		"language":     s.Language,
	}
}

// filterList trims downloads to display fields (no URLs or paths) and caps the count so
// the reply stays far below Chrome's 1 MB limit.
func filterList(raw json.RawMessage) any {
	var all []map[string]any
	_ = json.Unmarshal(raw, &all)
	const maxItems = 200
	if len(all) > maxItems {
		all = all[len(all)-maxItems:]
	}
	keep := []string{"id", "filename", "status", "size", "downloaded", "speed", "eta", "category"}
	out := make([]map[string]any, 0, len(all))
	for _, d := range all {
		m := map[string]any{}
		for _, k := range keep {
			if v, ok := d[k]; ok {
				m[k] = v
			}
		}
		out = append(out, m)
	}
	return out
}
