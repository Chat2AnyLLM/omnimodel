package omnicode

import (
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func setupLogging(verbose bool, logFile string) error {
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	var writers []io.Writer

	if logFile != "" {
		dir := filepath.Dir(logFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		writers = append(writers, f)
	} else if verbose {
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stderr})
	}

	if len(writers) == 0 {
		writers = append(writers, io.Discard)
	}

	log.Logger = zerolog.New(zerolog.MultiLevelWriter(writers...)).With().Timestamp().Logger()
	log.Info().Bool("verbose", verbose).Str("log_file", logFile).Msg("omnicode: logging initialized")
	return nil
}
