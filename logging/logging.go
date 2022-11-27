package logging

import (
	"io"
	"log"
	"os"

	"github.com/spf13/viper"

	"shipyard/version"
)

// Init registers a log writer that writes either to stdout or nowhere,
// depending on whether verbose flag is set.
func Init() {
	var logWriter io.Writer
	if viper.GetBool("verbose") {
		logWriter = os.Stdout
	} else {
		logWriter = io.Discard
	}

	log.SetOutput(logWriter)
	log.SetPrefix("SHIPYARD CLI\t")
	log.SetFlags(0)
	log.Println("Git commit:", version.GitCommit)
}
