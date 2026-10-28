//go:build !unix

package plugindev

import "os/exec"

func setProcessGroup(*exec.Cmd) {}

// interruptGroup kills outright: there is no portable SIGINT here, so skaffold skips cleanup.
func interruptGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}

func killGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}
