package main

import (
	"log/slog"
	"n8n/internal/config"
	"os"
)

type bootstrapped struct {
	cfg *config.Config
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	b := bootstrapped{
		cfg: cfg,
	}

	b.run()
}

func (b *bootstrapped) run() {

}
