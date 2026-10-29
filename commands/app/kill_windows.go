package app

import (
	"os"
	"os/exec"
	"strconv"
)

// killTree kills git and every process under it: a killed git leaves its ssh
// or credential helper running otherwise.
func killTree(pid int) error {
	if exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run() == nil {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
