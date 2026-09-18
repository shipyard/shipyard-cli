package server

import "testing"

func TestNegotiateProtocolVersion(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		want      string
	}{
		{
			name:      "echoes a supported version back",
			requested: "2025-06-18",
			want:      "2025-06-18",
		},
		{
			name:      "echoes an older supported version rather than forcing an upgrade",
			requested: "2024-11-05",
			want:      "2024-11-05",
		},
		{
			name:      "echoes the middle revision",
			requested: "2025-03-26",
			want:      "2025-03-26",
		},
		{
			name:      "answers an unknown future version with our latest",
			requested: "2099-01-01",
			want:      LatestProtocolVersion,
		},
		{
			name:      "answers an unknown past version with our latest",
			requested: "2024-01-01",
			want:      LatestProtocolVersion,
		},
		{
			name:      "treats an absent version as no preference",
			requested: "",
			want:      LatestProtocolVersion,
		},
		{
			name:      "treats garbage as no preference rather than failing the handshake",
			requested: "not-a-version",
			want:      LatestProtocolVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := negotiateProtocolVersion(tt.requested); got != tt.want {
				t.Errorf("negotiateProtocolVersion(%q) = %q, want %q", tt.requested, got, tt.want)
			}
		})
	}
}

// The latest must be in the supported set, and it must be first: the list is
// documented as newest first and the fallback depends on that being true.
func TestLatestProtocolVersionIsSupported(t *testing.T) {
	if len(supportedProtocolVersions) == 0 {
		t.Fatal("no supported protocol versions declared")
	}

	if supportedProtocolVersions[0] != LatestProtocolVersion {
		t.Errorf("supportedProtocolVersions[0] = %q, expected the latest %q",
			supportedProtocolVersions[0], LatestProtocolVersion)
	}

	seen := make(map[string]bool, len(supportedProtocolVersions))
	for _, v := range supportedProtocolVersions {
		if seen[v] {
			t.Errorf("duplicate protocol version %q", v)
		}
		seen[v] = true
	}
}
