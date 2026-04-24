package main

import (
	"log/slog"

	"pubsub/internal/hub"
	"pubsub/internal/server"
)

func main() {
	h := hub.NewHub()
	s := server.New(h, ":8080")
	slog.Info("starting server", "addr", ":8080")
	if err := s.Run(); err != nil {
		slog.Error("server failed", "err", err)
	}
}
