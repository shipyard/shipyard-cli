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

// clientInstructionLimit is where Claude Code (2.1.284, measured 2026-10-09)
// cuts server instructions off; nothing past it reaches the model.
const clientInstructionLimit = 2048

// Instructions returns the server instructions sent during initialize.
func Instructions() string {
	return strings.TrimSpace(serverInstructions)
}

// instructions is Instructions preceded, when this CLI is out of date, by a
// line asking the agent to suggest an upgrade. It goes first because clients
// truncate long instructions, and it is left out when it would push them past
// clientInstructionLimit: the rules matter more than the notice.
func (s *MCPServer) instructions() string {
	notice := staleNotice(version.Version, s.config.LatestVersion)
	if notice == "" || len(notice)+len("\n\n")+len(Instructions()) > clientInstructionLimit {
		return Instructions()
	}
	return notice + "\n\n" + Instructions()
}
