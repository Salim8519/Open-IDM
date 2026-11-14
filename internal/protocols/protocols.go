// Package protocols registers every downloader Peregrine ships (FTP, streams, video sites,
// BitTorrent) on an engine.Manager, so the desktop app, the CLI and test servers all get
// the same set. The engine core itself stays free of these dependencies.
package protocols

import (
	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/engine/proto/ftp"
	"github.com/Salim8519/Peregrine/engine/proto/stream"
)

// Options configures the downloaders that need outside tools or ports.
type Options struct {
	FFmpeg string // path to ffmpeg; "" = next to the executable, then PATH
}

// Register adds all downloaders to m. The returned function releases their resources
// (listening sockets, background clients); call it after m.Close.
func Register(m *engine.Manager, opts Options) (closeAll func()) {
	m.RegisterProtocol(ftp.New())
	so := stream.Options{FFmpeg: opts.FFmpeg}
	m.RegisterProtocol(stream.NewHLS(so))
	m.RegisterProtocol(stream.NewDASH(so))
	return func() {}
}
