//go:build !windows

package main

import "errors"

// detach is Windows-only; elsewhere the server runs in the calling terminal.
func detach([]string) error { return errors.New("not supported") }
