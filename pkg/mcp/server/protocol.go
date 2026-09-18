package server

// LatestProtocolVersion is the MCP revision this server prefers when a client
// asks for one it does not know.
const LatestProtocolVersion = "2025-06-18"

// supportedProtocolVersions lists every MCP revision this server can speak,
// newest first.
//
// The server implements tools, resources and prompts over stdio with text
// content, which all three revisions share. It does NOT implement JSON-RPC
// batching: processMessage decodes a single request, so a batched array fails to
// parse. Batching was optional in 2024-11-05 and 2025-03-26 and was removed in
// 2025-06-18, so the newest revision is the closest fit for what this server
// actually does. The two older ones stay supported for clients already speaking
// them, with that same batching gap they have always had.
//
//nolint:gochecknoglobals // A package-level table read by negotiateProtocolVersion.
var supportedProtocolVersions = []string{
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

// negotiateProtocolVersion implements the MCP lifecycle rule: respond with the
// version the client asked for when it is supported, otherwise respond with the
// newest one this server speaks and let the client decide whether to continue.
//
// An absent or unparsable version is treated as "no preference" and answered
// with the latest.
func negotiateProtocolVersion(requested string) string {
	for _, supported := range supportedProtocolVersions {
		if requested == supported {
			return requested
		}
	}

	return LatestProtocolVersion
}
