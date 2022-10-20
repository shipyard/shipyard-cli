package display

import (
	"fmt"
	"io"
	"os"
)

type Display interface {
	Output(...any)
	Fail(...any)
}

func NewSimpleDisplay() Display {
	return &simpleDisplay{writer: os.Stdout, errorWriter: os.Stderr}
}

type simpleDisplay struct {
	writer      io.Writer
	errorWriter io.Writer
}

func (sw *simpleDisplay) Output(a ...any) {
	fmt.Fprintln(sw.writer, a...)
}

func (sw *simpleDisplay) Fail(a ...any) {
	fmt.Fprintln(sw.errorWriter, "Error:", a)
}
