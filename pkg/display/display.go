package display

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

func New(writer, errorWriter io.Writer) Display {
	return Display{writer: writer, errorWriter: errorWriter}
}

type Display struct {
	writer      io.Writer
	errorWriter io.Writer
}

func (sw Display) Print(a any) error {
	_, err := fmt.Fprint(sw.writer, a)
	return err
}

func (sw Display) Println(a any) error {
	_, err := fmt.Fprintln(sw.writer, a)
	return err
}

func (sw Display) Fail(a any) error {
	red := color.New(color.FgRed)
	_, err := red.Fprint(sw.errorWriter, "Error:", a)
	return err
}

func Print(a any) {
	_, _ = fmt.Fprintf(os.Stdout, "%s", a)
}

func Println(a any) {
	_, _ = fmt.Fprintf(os.Stdout, "%s\n", a)
}

func Fail(a any) {
	red := color.New(color.FgRed)
	_, _ = red.Fprint(os.Stderr, "Error:", a)
}
