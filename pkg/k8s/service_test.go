package k8s

import (
	"strings"
	"sync"
	"testing"
)

// The cap exists to keep one `cat` of a log file out of a model's context. It
// has to hold in the middle of a write, not just on whole writes, and it must
// never report a short write: the stream copy treats that as an error and aborts
// the exec, losing everything the command printed.
func TestCappedBufferHoldsTheLimit(t *testing.T) {
	tests := []struct {
		name          string
		limit         int
		writes        []string
		want          string
		wantTruncated bool
	}{
		{
			name:   "under the limit passes through",
			limit:  10,
			writes: []string{"abc", "def"},
			want:   "abcdef",
		},
		{
			name:          "a write straddling the limit is cut at it",
			limit:         4,
			writes:        []string{"ab", "cdef"},
			want:          "abcd",
			wantTruncated: true,
		},
		{
			name:          "writes after the limit are dropped",
			limit:         3,
			writes:        []string{"abc", "def", "ghi"},
			want:          "abc",
			wantTruncated: true,
		},
		{
			name:          "a multi-byte character is not split at the limit",
			limit:         4,
			writes:        []string{"ab", "éé"},
			want:          "abé",
			wantTruncated: true,
		},
		{
			name:          "nothing lands after the cut, even with room left",
			limit:         4,
			writes:        []string{"abc", "é", "d"},
			want:          "abc",
			wantTruncated: true,
		},
		{
			name:   "no limit keeps everything",
			limit:  0,
			writes: []string{"abc", "def", "ghi"},
			want:   "abcdefghi",
		},
		{
			name:          "exactly at the limit is not truncated",
			limit:         6,
			writes:        []string{"abc", "def"},
			want:          "abcdef",
			wantTruncated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &cappedBuffer{limit: tt.limit}

			for _, chunk := range tt.writes {
				n, err := w.Write([]byte(chunk))
				if err != nil {
					t.Fatalf("Write(%q) returned an error: %v", chunk, err)
				}
				// Every write must be reported as fully accepted.
				if n != len(chunk) {
					t.Errorf("Write(%q) = %d, want %d: a short write aborts the exec", chunk, n, len(chunk))
				}
			}

			if got := w.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if got := w.Truncated(); got != tt.wantTruncated {
				t.Errorf("Truncated() = %v, want %v", got, tt.wantTruncated)
			}
		})
	}
}

// A timed-out exec reads the buffer while the stream copy is still writing to
// it. Run with -race: unguarded, this is the bug that would surface in
// production and never in a normal test run.
func TestCappedBufferIsSafeForConcurrentUse(t *testing.T) {
	w := &cappedBuffer{limit: 1024}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = w.Write([]byte("chunk"))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = w.String()
			_ = w.Truncated()
		}
	}()

	wg.Wait()

	if got := w.String(); !strings.HasPrefix(got, "chunk") {
		t.Errorf("expected the buffer to hold what was written, got %q", got)
	}
}
