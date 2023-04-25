package display

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

func New() *Display {
	return &Display{writer: os.Stdout, errorWriter: os.Stderr}
}

type Display struct {
	writer      io.Writer
	errorWriter io.Writer
}

func (sw *Display) Print(a ...any) {
	_, _ = fmt.Fprint(sw.writer, a...)
}

func (sw *Display) Println(a ...any) {
	_, _ = fmt.Fprintln(sw.writer, a...)
}

func (sw *Display) Fail(a ...any) {
	red := color.New(color.FgRed)
	_, _ = red.Fprint(sw.errorWriter, "Error:", a)
}
