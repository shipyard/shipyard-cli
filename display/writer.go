package display

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

func New() *simpleDisplay {
	return &simpleDisplay{writer: os.Stdout, errorWriter: os.Stderr}
}

type simpleDisplay struct {
	writer      io.Writer
	errorWriter io.Writer
}

func (sw *simpleDisplay) Print(a ...any) {
	fmt.Fprint(sw.writer, a...)
}

func (sw *simpleDisplay) Println(a ...any) {
	fmt.Fprintln(sw.writer, a...)
}

func (sw *simpleDisplay) Fail(a ...any) {
	red := color.New(color.FgRed)
	red.Fprint(sw.errorWriter, "Error:", a)
}
