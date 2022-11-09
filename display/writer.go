package display

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
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
	fmt.Fprint(sw.writer, a...)
}

func (sw *simpleDisplay) Fail(a ...any) {
	red := color.New(color.FgRed)
	red.Fprint(sw.errorWriter, "Error:", a)
}
