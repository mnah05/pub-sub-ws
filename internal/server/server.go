package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"pubsub/internal/client"
	"pubsub/internal/hub"
	"pubsub/pkg"
)

type Server struct {
	Hub  *hub.Hub
	Addr string
}

func New(h *hub.Hub, addr string) *Server {
	return &Server{Hub: h, Addr: addr}
}

func (s *Server) Run() error {
	r := chi.NewRouter()
	r.Get("/ws", s.wsHandler)
	r.Get("/topics", s.topicHandler)
	r.Get("/clients", s.clientHandler)

	srv := &http.Server{
		Addr:    s.Addr,
		Handler: r,
	}

	go func() {
		slog.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	slog.Info("server stopped")
	return nil
}

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("upgrade failed", "err", err)
		return
	}

	c := client.New(conn, s.Hub)
	slog.Info("client connected", "id", c.Id)

	ack := pkg.SystemResponse{
		Type:     "system",
		ClientID: c.Id,
		Message:  "connected",
	}
	data, _ := json.Marshal(ack)
	conn.Write(r.Context(), websocket.MessageText, data)

	ctx, cancel := context.WithCancel(context.Background())

	go client.ReadLoop(c, s.Hub, ctx, cancel)
	go client.WriteLoop(c, ctx, cancel)
}

func (s *Server) topicHandler(w http.ResponseWriter, r *http.Request) {
	topics := s.Hub.ListTopics()
	resp := pkg.TopicsListResponse{
		Type:   "topics",
		Topics: topics,
		Count:  len(topics),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) clientHandler(w http.ResponseWriter, r *http.Request) {
	clients := s.Hub.ListClients()
	resp := pkg.ClientsListResponse{
		Type:    "clients",
		Clients: clients,
		Count:   len(clients),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
