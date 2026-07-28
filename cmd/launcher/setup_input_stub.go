//go:build !windows

package main

import (
	"io"
	"os"
)

func consoleInput() io.Reader { return os.Stdin }
