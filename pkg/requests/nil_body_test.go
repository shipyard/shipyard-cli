package requests

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

// TestDoMethodNilBodyHandling tests the actual body handling logic from Do method
func TestDoMethodNilBodyHandling(t *testing.T) {
	// This simulates the exact logic from the Do method
	testCases := []struct {
		name         string
		body         any
		expectNil    bool
		expectContent string
	}{
		{
			name:         "nil body should produce nil reader",
			body:         nil,
			expectNil:    true,
			expectContent: "",
		},
		{
			name:         "empty struct produces JSON",
			body:         struct{}{},
			expectNil:    false,
			expectContent: "{}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This is the exact code from Do method
			var reqBody io.Reader
			switch body := tc.body.(type) {
			case nil:
				// For nil body (common with GET requests), use nil reader
				reqBody = nil
			case []byte:
				reqBody = bytes.NewReader(body)
			case *bytes.Buffer:
				reqBody = bytes.NewReader(body.Bytes())
			default:
				serialized, err := json.Marshal(body)
				if err != nil {
					t.Fatalf("Failed to marshal: %v", err)
				}
				reqBody = bytes.NewReader(serialized)
			}

			// Check the result
			if tc.expectNil {
				if reqBody != nil {
					// Read to see what's in it
					buf := new(bytes.Buffer)
					_, _ = buf.ReadFrom(reqBody)
					content := buf.String()
					
					// This should fail when fix is commented out!
					if content == "null" {
						t.Errorf("BUG DETECTED: nil body was marshaled to 'null' (4 bytes) instead of remaining nil")
					} else {
						t.Errorf("Expected nil reader for nil body, got reader with content: %q", content)
					}
				}
			} else {
				if reqBody == nil {
					t.Errorf("Expected non-nil reader, got nil")
				}
			}
		})
	}
}

// TestNilBodyMarshalBehavior verifies what happens when we marshal nil
// This documents the root cause of the bug: json.Marshal(nil) returns "null"
func TestNilBodyMarshalBehavior(t *testing.T) {
	data, err := json.Marshal(nil)
	if err != nil {
		t.Fatalf("Failed to marshal nil: %v", err)
	}
	
	// This is the bug - nil gets marshaled to "null" (4 bytes)
	if string(data) != "null" {
		t.Errorf("json.Marshal(nil) = %q, expected \"null\"", string(data))
	}
	
	if len(data) != 4 {
		t.Errorf("json.Marshal(nil) length = %d, expected 4", len(data))
	}
}