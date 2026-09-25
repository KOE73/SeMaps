package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detach starts this binary again with --here in a new console window.
func detach(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, append([]string{"--here"}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010} // CREATE_NEW_CONSOLE
	return cmd.Start()
}

func startBackgroundHost(projectFile string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--here", "--no-browser", projectFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd.Stdout, cmd.Stderr = devnull, devnull
	return cmd.Start()
}
