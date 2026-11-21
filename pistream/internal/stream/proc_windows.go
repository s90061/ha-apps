//go:build windows

package stream

import "os/exec"

func suspendProcess(cmd *exec.Cmd) error {
	return nil
}

func resumeProcess(cmd *exec.Cmd) error {
	return nil
}
