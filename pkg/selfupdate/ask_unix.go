//go:build !windows

package selfupdate

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// AskTerminal reads one answer from a terminal, giving up after the timeout.
//
// It waits with poll and reads only once input is ready. A reader goroutine
// abandoned on timeout would stay blocked on stdin and take the next line the
// user typed, or the keystrokes of a later `shipyard exec`. A terminal in
// canonical mode reports ready only once a whole line is in, so one read
// returns the full answer.
func AskTerminal(f *os.File) func(time.Duration) (string, error) {
	return func(timeout time.Duration) (string, error) {
		deadline := time.Now().Add(timeout)
		fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}
		for {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return "", ErrNoAnswer
			}
			n, err := unix.Poll(fds, int(remaining.Milliseconds())+1)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil {
				return "", err
			}
			if n == 0 {
				return "", ErrNoAnswer
			}
			break
		}
		buf := make([]byte, 256)
		n, err := f.Read(buf)
		return normalizeAnswer(string(buf[:n]), err), nil
	}
}
