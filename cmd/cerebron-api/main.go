package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"cerebron/internal/app"
	"cerebron/internal/config"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		panic(err)
	}

	application := app.New(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		os.Exit(1)
	}
}
