//go:build unix

package serve

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 200 * time.Millisecond
}

// stopListening terminates a llama-server left listening on port by an earlier process.
func stopListening(port int) {
	if port <= 0 {
		return
	}
	pids := listenPIDs(port)
	signaled := false
	for _, pid := range pids {
		if isLlamaServer(pid) {
			_ = syscall.Kill(pid, syscall.SIGTERM)
			signaled = true
		}
	}
	if !signaled {
		return
	}
	time.Sleep(300 * time.Millisecond)
	for _, pid := range listenPIDs(port) {
		if isLlamaServer(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

func listenPIDs(port int) []int {
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(field)
		if err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

func isLlamaServer(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.TrimSpace(string(out)), "llama-server")
}
