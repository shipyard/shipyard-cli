package server

import (
	_ "embed"
	"strings"

	"github.com/shipyard/shipyard-cli/version"
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

// instructions is Instructions preceded, when this CLI is out of date, by a
// line asking the agent to suggest an upgrade. It goes first because clients
// truncate long instructions: Claude Code cuts this text off partway through.
func (s *MCPServer) instructions() string {
	if notice := staleNotice(version.Version, s.config.LatestVersion); notice != "" {
		return notice + "\n\n" + Instructions()
	}
	return Instructions()
}
