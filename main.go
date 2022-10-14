package main

import (
	"fmt"
	"os"

	"shipyard/cmd"
)

func main() {
	// Handle a panic.
	defer func() {
		if err := recover(); err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}

	}()
	cmd.Execute()
}
