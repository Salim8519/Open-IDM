// Package protocols registers every downloader Peregrine ships (FTP, streams, video sites,
// BitTorrent) on an engine.Manager, so the desktop app, the CLI and test servers all get
// the same set. The engine core itself stays free of these dependencies.
package protocols

import (
	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/engine/proto/ftp"
)

// Options configures the downloaders that need outside tools or ports.
type Options struct{}

// Register adds all downloaders to m. The returned function releases their resources
// (listening sockets, background clients); call it after m.Close.
func Register(m *engine.Manager, opts Options) (closeAll func()) {
	m.RegisterProtocol(ftp.New())
	return func() {}
}
