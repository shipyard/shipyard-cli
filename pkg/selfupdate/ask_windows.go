//go:build windows

package selfupdate

import (
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// AskTerminal reads one answer from the console, giving up after the timeout.
// It waits on the console handle and reads only once input is pending, so
// nothing is left blocked on stdin after a timeout.
func AskTerminal(f *os.File) func(time.Duration) (string, error) {
	return func(timeout time.Duration) (string, error) {
		ev, err := windows.WaitForSingleObject(windows.Handle(f.Fd()), uint32(timeout.Milliseconds()))
		if err != nil {
			return "", err
		}
		if ev == uint32(windows.WAIT_TIMEOUT) {
			return "", ErrNoAnswer
		}
		buf := make([]byte, 256)
		n, err := f.Read(buf)
		return normalizeAnswer(string(buf[:n]), err), nil
	}
}
