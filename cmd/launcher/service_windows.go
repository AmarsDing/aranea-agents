//go:build windows

package main

import (
	"fmt"
	"io"
	"time"

	"golang.org/x/sys/windows/svc"
)

func isWindowsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

// runAsService dispatches the SCM control loop; blocks until service stop.
func runAsService(root string) {
	if err := svc.Run(serviceName, &araneaService{root: root}); err != nil {
		_, logf := openLauncherLog(root)
		_, _ = io.WriteString(logf, "svc.Run failed: "+err.Error()+"\n")
		_ = logf.Close()
	}
}

type araneaService struct{ root string }

func (s *araneaService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	_, logf := openLauncherLog(s.root)
	defer logf.Close()
	logger := func(format string, a ...any) {
		_, _ = io.WriteString(logf, fmt.Sprintf("%s [svc] %s\n",
			time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, a...)))
	}
	logger("service starting (root=%s)", s.root)

	// Retry: delayed-auto may still race PostgreSQL service startup on boot.
	var startErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if startErr = startStack(s.root, nil, logger, true); startErr == nil {
			break
		}
		logger("startStack attempt %d failed: %v", attempt, startErr)
		time.Sleep(time.Duration(attempt*10) * time.Second)
	}
	if startErr != nil {
		logger("service start failed permanently: %v", startErr)
		return false, 1
	}

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for c := range r {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			logger("service stop requested")
			_ = stopAll(s.root, logger)
			return false, 0
		case svc.Interrogate:
			changes <- c.CurrentStatus
		}
	}
	return false, 0
}
