//go:build !unix

package compile

import (
	"os/exec"
	"time"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = 200 * time.Millisecond
}
