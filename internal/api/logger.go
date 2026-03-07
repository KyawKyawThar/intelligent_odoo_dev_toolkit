package api

import (
	"os"

	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	log = zerolog.New(os.Stderr).With().Timestamp().Logger()
}
