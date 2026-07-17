//go:build !windows

package main

import "fmt"

type statusConsole struct{}

func openStatusConsole(title string) *statusConsole {
	fmt.Println(title)
	return &statusConsole{}
}

func (c *statusConsole) Println(s string) {
	if c == nil {
		return
	}
	fmt.Println(s)
}

func (c *statusConsole) Printf(format string, args ...any) {
	if c == nil {
		return
	}
	fmt.Printf(format+"\n", args...)
}

func (c *statusConsole) WaitDismiss(prompt string) {
	if c == nil {
		return
	}
	if prompt != "" {
		fmt.Println(prompt)
	}
}

func (c *statusConsole) Close() {}
