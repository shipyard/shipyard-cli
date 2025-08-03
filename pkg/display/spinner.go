package display

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// Spinner represents a terminal spinner animation
type Spinner struct {
	mu       sync.Mutex
	writer   io.Writer
	message  string
	active   bool
	stopCh   chan struct{}
	frames   []string
	interval time.Duration
}

// NewSpinner creates a new spinner with the given message
func NewSpinner(message string) *Spinner {
	return &Spinner{
		writer:   os.Stdout,
		message:  message,
		frames:   []string{"*", "⋆", "✦", "✧", "✦", "⋆"},
		interval: 150 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		return
	}

	// Only show spinner if we're in a terminal
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		// For non-terminal output, just print the message once
		fmt.Fprintf(s.writer, "%s\n", s.message)
		return
	}

	s.active = true
	s.stopCh = make(chan struct{})

	go s.animate()
}

// Stop ends the spinner animation and clears the line
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return
	}

	s.active = false
	close(s.stopCh)

	// Clear the line if we're in a terminal
	if isatty.IsTerminal(os.Stdout.Fd()) {
		// Simply clear the current line and move cursor to beginning
		fmt.Fprint(s.writer, "\r\033[K")
	}
}

// animate runs the spinner animation loop
func (s *Spinner) animate() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	frameIndex := 0
	cyan := color.New(color.FgCyan)

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			frame := s.frames[frameIndex%len(s.frames)]
			fmt.Fprintf(s.writer, "\r%s %s", cyan.Sprint(frame), s.message)
			frameIndex++
		}
	}
}

// SetMessage updates the spinner message while it's running
func (s *Spinner) SetMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}