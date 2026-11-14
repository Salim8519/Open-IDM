//go:build unix

package video

import (
	"os/exec"
	"syscall"
)

// procTree runs yt-dlp in its own process group, so cancelling kills yt-dlp and the ffmpeg
// processes it started.
type procTree struct{}

func newProcTree(cmd *exec.Cmd) *procTree {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
	return &procTree{}
}

func (*procTree) started(*exec.Cmd) {}
func (*procTree) close()            {}
