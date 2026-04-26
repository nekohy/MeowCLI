package main

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nekohy/MeowCLI/internal/app"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().Timestamp().Logger()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// pprof debug server, only enabled when DEBUG=TRUE
	if strings.EqualFold(os.Getenv("DEBUG"), "true") {
		go func() {
			log.Info().Msg("pprof listening on :6060")
			if err := http.ListenAndServe(":6060", nil); err != nil {
				log.Error().Err(err).Msg("pprof server failed")
			}
		}()
	}

	if err := app.Run(ctx, app.LoadConfig()); err != nil {
		log.Fatal().Err(err).Msg("run app")
	}
}
