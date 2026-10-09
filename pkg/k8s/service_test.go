package k8s

import (
	"strings"
	"sync"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func pod(name string, ready bool, restarts int32) v1.Pod {
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
			{Ready: ready, RestartCount: restarts},
		}},
	}
}

// get_logs previous should read the pod that is crashing, not whichever pod the API lists first.
func TestPickCrashedPod(t *testing.T) {
	tests := []struct {
		name string
		pods []v1.Pod
		want string
	}{
		{"not ready beats ready", []v1.Pod{pod("a", true, 9), pod("b", false, 1)}, "b"},
		{"most restarts among not ready", []v1.Pod{pod("a", false, 1), pod("b", false, 4), pod("c", false, 2)}, "b"},
		{"most restarts among ready", []v1.Pod{pod("a", true, 0), pod("b", true, 3)}, "b"},
		{"first on a tie", []v1.Pod{pod("a", false, 2), pod("b", false, 2)}, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pickCrashedPod(tt.pods)
			if !ok || got.Name != tt.want {
				t.Fatalf("got %q, want %q", got.Name, tt.want)
			}
		})
	}
	if _, ok := pickCrashedPod(nil); ok {
		t.Fatal("no pod expected")
	}
}

func TestContainerState(t *testing.T) {
	crashed := pod("web-1", false, 5)
	crashed.Status.ContainerStatuses[0].LastTerminationState.Terminated = &v1.ContainerStateTerminated{
		Reason: "OOMKilled", ExitCode: 137,
	}
	crashed.Status.ContainerStatuses[0].State.Waiting = &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}

	state := containerState(crashed)
	if state.Pod != "web-1" || state.Ready || state.RestartCount != 5 || state.Reason != "OOMKilled" ||
		state.ExitCode == nil || *state.ExitCode != 137 {
		t.Fatalf("unexpected state %+v", state)
	}

	waiting := pod("web-2", false, 0)
	waiting.Status.ContainerStatuses[0].State.Waiting = &v1.ContainerStateWaiting{Reason: "ImagePullBackOff"}
	if state := containerState(waiting); state.Reason != "ImagePullBackOff" || state.ExitCode != nil {
		t.Fatalf("unexpected state %+v", state)
	}

	if state := containerState(v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pending"}}); state.Ready {
		t.Fatalf("a pod with no container status is not ready: %+v", state)
	}
}
