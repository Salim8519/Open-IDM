//go:build windows

package video

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procTree runs yt-dlp without a console window, inside a job object, so cancelling kills
// yt-dlp and everything it started (the PyInstaller child process, ffmpeg).
type procTree struct {
	mu  sync.Mutex
	job windows.Handle
}

func newProcTree(cmd *exec.Cmd) *procTree {
	t := &procTree{}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	cmd.Cancel = func() error {
		t.mu.Lock()
		if t.job != 0 {
			_ = windows.TerminateJobObject(t.job, 1)
		}
		t.mu.Unlock()
		return cmd.Process.Kill()
	}
	return t
}

// started puts the process into a kill-on-close job. Failure is not fatal: Kill still
// stops yt-dlp itself.
func (t *procTree) started(cmd *exec.Cmd) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	t.mu.Lock()
	t.job = job
	t.mu.Unlock()
}

// close releases the job; KILL_ON_JOB_CLOSE ends any helper still running.
func (t *procTree) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		_ = windows.CloseHandle(t.job)
		t.job = 0
	}
}
