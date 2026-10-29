//go:build !unix

package serve

import (
	"os/exec"
	"time"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = 200 * time.Millisecond
}

func stopListening(int) {}
