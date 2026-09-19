//go:build windows

package chrome

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var backgroundJobMu sync.Mutex
var backgroundJobs = make(map[int]windows.Handle)

// configureBackgroundCommand prevents Windows from creating a visible console
// window for helper processes used by silent Chrome/session work. Interactive
// login Chrome is intentionally not passed through this helper.
func configureBackgroundCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// startBackgroundCommand starts a silent Chrome process inside a Windows Job
// Object. KILL_ON_JOB_CLOSE guarantees that a browser tree cannot outlive
// this application after an unexpected exit.
func startBackgroundCommand(cmd *exec.Cmd) error {
	if cmd == nil {
		return fmt.Errorf("background command is nil")
	}
	configureBackgroundCommand(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		KillProcessTree(cmd.Process.Pid)
		return fmt.Errorf("create Chrome lifetime job: %w", err)
	}

	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		_ = windows.CloseHandle(job)
		KillProcessTree(cmd.Process.Pid)
		return fmt.Errorf("configure Chrome lifetime job: %w", err)
	}

	var assignErr error
	if err := cmd.Process.WithHandle(func(handle uintptr) {
		assignErr = windows.AssignProcessToJobObject(job, windows.Handle(handle))
	}); err != nil {
		assignErr = err
	}
	if assignErr != nil {
		_ = windows.CloseHandle(job)
		KillProcessTree(cmd.Process.Pid)
		return fmt.Errorf("assign Chrome to lifetime job: %w", assignErr)
	}

	backgroundJobMu.Lock()
	backgroundJobs[cmd.Process.Pid] = job
	backgroundJobMu.Unlock()
	return nil
}

func forgetBackgroundJob(pid int) {
	backgroundJobMu.Lock()
	job, ok := backgroundJobs[pid]
	if ok {
		delete(backgroundJobs, pid)
	}
	backgroundJobMu.Unlock()
	if ok {
		_ = windows.CloseHandle(job)
	}
}
