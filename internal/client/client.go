package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/coder/websocket"

	"pubsub/internal/handler"
	"pubsub/internal/hub"
)

var idCounter atomic.Uint64

func New(conn *websocket.Conn, h *hub.Hub) *hub.Client {
	id := fmt.Sprintf("%d", idCounter.Add(1))
	c := &hub.Client{
		Id:     id,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		Topics: make(map[string]*hub.Topic),
	}
	h.AddClient(c)
	return c
}

func ReadLoop(c *hub.Client, h *hub.Hub, ctx context.Context, cancel context.CancelFunc) {
	defer Cleanup(c, h)

	for {
		_, msg, err := c.Conn.Read(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Warn("read error", "id", c.Id, "err", err)
			}
			cancel()
			return
		}

		handler.Dispatch(c, h, msg)
	}
}

func WriteLoop(c *hub.Client, ctx context.Context, cancel context.CancelFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.Send:
			if !ok {
				return
			}
			c.Mu.Lock()
			err := c.Conn.Write(ctx, websocket.MessageText, msg)
			c.Mu.Unlock()
			if err != nil {
				slog.Warn("write error", "id", c.Id, "err", err)
				cancel()
				return
			}
		}
	}
}

func Cleanup(c *hub.Client, h *hub.Hub) {
	h.RemoveClient(c)
	c.Conn.Close(websocket.StatusNormalClosure, "disconnect")
	slog.Info("client disconnected", "id", c.Id)
}
