package logging

import (
	"io"
	"log"

	"github.com/spf13/viper"
)

func Init(w io.Writer, prefix string) {
	log.SetOutput(w)
	log.SetPrefix(prefix)
}

func LogIfVerbose(messages ...any) {
	if verbose := viper.GetBool("verbose"); verbose {
		log.Println(messages...)
	}
}
