//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

// detach is Windows-only; elsewhere the server runs in the calling terminal.
func detach([]string) error { return errors.New("not supported") }

func startBackgroundHost(projectFile string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--here", "--no-browser", projectFile)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Start()
}
