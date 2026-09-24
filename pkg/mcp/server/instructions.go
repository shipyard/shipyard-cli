package server

import (
	_ "embed"
	"strings"
)

// serverInstructions is returned in the initialize result, where clients surface
// it to the model as guidance about this server.
//
// It is deliberately a summary rather than the full verification loop: this text
// lands in every session for every client that reads it, so the loop itself
// stays in the verify prompt, which is fetched only when needed.
//
//go:embed instructions.md
var serverInstructions string

// Instructions returns the server instructions sent during initialize.
func Instructions() string {
	return strings.TrimSpace(serverInstructions)
}
