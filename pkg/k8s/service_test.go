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
		Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "app"}}},
		Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
			{Name: "app", Ready: ready, RestartCount: restarts},
		}},
	}
}

// pending is a pod the scheduler hasn't started yet: no container statuses at all.
func pending(name string) v1.Pod {
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "app"}}},
		Status:     v1.PodStatus{Phase: v1.PodPending},
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
		// During a rollout the new pod isn't ready yet, but it has no previous run to read
		{"a starting pod does not beat one that crashed", []v1.Pod{pod("old", true, 12), pending("new")}, "old"},
		{"a not-ready pod with no restarts does not beat one that crashed", []v1.Pod{pod("old", true, 3), pod("new", false, 0)}, "old"},
		{"not ready still wins when nothing restarted", []v1.Pod{pod("a", true, 0), pod("b", false, 0)}, "b"},
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

// Status order is not spec order. With a sidecar, the state must describe the
// container whose logs are read: the pod's default container, found by name.
func TestContainerStateFollowsTheDefaultContainer(t *testing.T) {
	p := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1"},
		Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "app"}, {Name: "sidecar"}}},
		Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
			{Name: "sidecar", Ready: true},
			{Name: "app", Ready: false, RestartCount: 3},
		}},
	}
	if state := containerState(p); state.Container != "app" || state.RestartCount != 3 || state.Ready {
		t.Fatalf("expected the app container's state, got %+v", state)
	}

	// Nothing crashed: the annotated default container
	p.Status.ContainerStatuses[1].RestartCount = 0
	p.Annotations = map[string]string{defaultContainerAnnotation: "sidecar"}
	if state := containerState(p); state.Container != "sidecar" || state.RestartCount != 0 || !state.Ready {
		t.Fatalf("expected the annotated container's state, got %+v", state)
	}
}

func TestDefaultContainer(t *testing.T) {
	spec := v1.PodSpec{Containers: []v1.Container{{Name: "app"}, {Name: "sidecar"}}}
	tests := []struct {
		name       string
		annotation string
		want       string
	}{
		{"first spec container", "", "app"},
		{"annotation names another container", "sidecar", "sidecar"},
		{"annotation naming no container is ignored", "missing", "app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := v1.Pod{Spec: spec}
			if tt.annotation != "" {
				p.Annotations = map[string]string{defaultContainerAnnotation: tt.annotation}
			}
			if got := defaultContainer(p); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	if got := defaultContainer(v1.Pod{}); got != "" {
		t.Fatalf("a pod with no containers has no default, got %q", got)
	}
}

// A crash in an init container (migrations, say) leaves the app container waiting with no
// restarts. The state must name the init container that crashed, so its logs are the ones read.
func TestContainerStateFindsACrashedInitContainer(t *testing.T) {
	p := pod("web-1", false, 0)
	p.Status.ContainerStatuses[0].State.Waiting = &v1.ContainerStateWaiting{Reason: "PodInitializing"}
	p.Spec.InitContainers = []v1.Container{{Name: "migrate"}}
	p.Status.InitContainerStatuses = []v1.ContainerStatus{{
		Name: "migrate", RestartCount: 4,
		LastTerminationState: v1.ContainerState{Terminated: &v1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
	}}

	state := containerState(p)
	if state.Container != "migrate" || !state.Init || state.RestartCount != 4 || state.Reason != "Error" ||
		state.ExitCode == nil || *state.ExitCode != 1 {
		t.Fatalf("expected the crashed init container, got %+v", state)
	}
}

// A crashing sidecar is reported when the default container is healthy; a crashing
// default container wins over a crashing sidecar.
func TestContainerStateFindsACrashedSidecar(t *testing.T) {
	p := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1"},
		Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "app"}, {Name: "proxy"}}},
		Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
			{Name: "app", Ready: true},
			{Name: "proxy", RestartCount: 2},
		}},
	}
	if state := containerState(p); state.Container != "proxy" || state.Init || state.RestartCount != 2 {
		t.Fatalf("expected the crashed sidecar, got %+v", state)
	}

	p.Status.ContainerStatuses[0].RestartCount = 1
	if state := containerState(p); state.Container != "app" {
		t.Fatalf("a crashed default container should win, got %+v", state)
	}
}

// A container that exited and hasn't restarted yet has its stop reason in State, not LastTerminationState.
func TestContainerStateReadsATerminatedContainer(t *testing.T) {
	p := pod("job-1", false, 0)
	p.Status.ContainerStatuses[0].State.Terminated = &v1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}
	if state := containerState(p); state.Reason != "Error" || state.ExitCode == nil || *state.ExitCode != 1 {
		t.Fatalf("unexpected state %+v", state)
	}
}

// UseCrashedPod must move the Service itself onto the crashed pod and container, or the
// previous-run logs would come from a different pod than the header describes.
func TestUseCrashedPodSwitchesPodAndContainer(t *testing.T) {
	healthy := pod("a", true, 0)
	crashed := pod("b", false, 4)
	crashed.Spec.Containers = []v1.Container{{Name: "worker"}}
	crashed.Status.ContainerStatuses[0].Name = "worker"
	s := &Service{pod: "a", container: "app", pods: []v1.Pod{healthy, crashed}}

	state := s.UseCrashedPod()
	if s.pod != "b" || s.container != "worker" || state.Pod != "b" || state.Container != "worker" || state.RestartCount != 4 {
		t.Fatalf("pod=%q container=%q state=%+v", s.pod, s.container, state)
	}

	// The crashed container is read, even when it isn't the pod's default
	initCrash := pod("c", false, 0)
	initCrash.Status.InitContainerStatuses = []v1.ContainerStatus{{Name: "migrate", RestartCount: 2}}
	s = &Service{pod: "c", container: "app", pods: []v1.Pod{initCrash}}
	if state := s.UseCrashedPod(); s.container != "migrate" || state.Container != "migrate" {
		t.Fatalf("container=%q state=%+v", s.container, state)
	}

	empty := &Service{pod: "x", container: "c"}
	if state := empty.UseCrashedPod(); state.Pod != "x" || state.Container != "c" || empty.pod != "x" || empty.container != "c" {
		t.Fatalf("with no pods listed the Service should stay put: %+v", state)
	}
}
