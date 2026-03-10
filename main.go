package main

import (
	"context"
	"os"
	"os/signal"
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

	if err := app.Run(ctx, app.LoadConfig()); err != nil {
		log.Fatal().Err(err).Msg("run app")
	}
}
