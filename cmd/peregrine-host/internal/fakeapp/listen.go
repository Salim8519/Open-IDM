package main

import (
	"net"
	"net/http"
	"time"
)

type server struct{ port int }

func listen(h http.Handler) (*server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return &server{port: ln.Addr().(*net.TCPAddr).Port}, nil
}
