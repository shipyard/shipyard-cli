package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/fatih/color"

	"github.com/shipyard/shipyard-cli/commands"
)

func main() {
	// Handle a panic.
	defer func() {
		if err := recover(); err != nil {
			red := color.New(color.FgHiRed)
			_, _ = red.Fprintf(os.Stderr, "Runtime error: %v\n", err)
			_, _ = fmt.Fprintln(os.Stderr, string(debug.Stack()))
			os.Exit(1)
		}
	}()
	commands.Execute()
}
