package main

import (
	"os"
	"strings"
)

func main() {
	path := os.Args[1]
	old := os.Args[2]
	repl := os.Args[3]
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	result := strings.ReplaceAll(string(data), old, repl)
	if err := os.WriteFile(path, []byte(result), 0); err != nil {
		panic(err)
	}
}
