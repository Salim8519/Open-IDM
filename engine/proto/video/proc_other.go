//go:build !unix && !windows

package video

import "os/exec"

type procTree struct{}

func newProcTree(*exec.Cmd) *procTree { return &procTree{} }
func (*procTree) started(*exec.Cmd)   {}
func (*procTree) close()              {}
