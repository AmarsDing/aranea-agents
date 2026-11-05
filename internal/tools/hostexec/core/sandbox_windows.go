//go:build windows

//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package hostexec

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	createNewProcessGroup    = 0x00000200
	jobActiveProcessLimit    = 64
	processSetQuotaTerminate = windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE
)

type windowsJobSandbox struct {
	job windows.Handle
}

func (s *windowsJobSandbox) Kill() error {
	if s == nil || s.job == 0 {
		return nil
	}
	return windows.TerminateJobObject(s.job, 1)
}

func (s *windowsJobSandbox) Close() error {
	if s == nil || s.job == 0 {
		return nil
	}
	err := windows.CloseHandle(s.job)
	s.job = 0
	return err
}

func wrapLinuxSandbox(cmd *exec.Cmd, _ string) *exec.Cmd {
	return cmd
}

func prepareOSSandbox(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	attr := cmd.SysProcAttr
	if attr == nil {
		attr = &syscall.SysProcAttr{}
		cmd.SysProcAttr = attr
	}
	attr.CreationFlags |= createNewProcessGroup
}

func attachProcessSandbox(cmd *exec.Cmd) (processSandbox, error) {
	if cmd == nil || cmd.Process == nil {
		return nil, fmt.Errorf("process sandbox: command has not started")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("process sandbox: create job object: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
		windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	info.BasicLimitInformation.ActiveProcessLimit = jobActiveProcessLimit
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("process sandbox: set job limits: %w", err)
	}
	proc, err := windows.OpenProcess(processSetQuotaTerminate, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("process sandbox: open process: %w", err)
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("process sandbox: assign job: %w", err)
	}
	return &windowsJobSandbox{job: job}, nil
}
