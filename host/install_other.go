//go:build !windows

package main

import "errors"

// Self-install is Windows-only for now: elsewhere put the binary on PATH by hand.
func install() error  { return errors.New("self-install is Windows-only; put semaps on PATH yourself") }
func installed() bool { return true }
func offerInstall()   {}
