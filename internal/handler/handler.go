package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coder/websocket"
	"github.com/go-playground/validator/v10"

	"pubsub/internal/hub"
	"pubsub/pkg"
)

func Dispatch(c *hub.Client, h *hub.Hub, raw []byte) {
	var base struct {
		Action string `json:"action" validate:"required,oneof=subscribe unsubscribe publish ping"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		sendError(c, "", "invalid json")
		return
	}

	if err := pkg.Validate.Struct(base); err != nil {
		sendError(c, base.Action, validationMsg(err))
		return
	}

	switch base.Action {
	case "subscribe":
		handleSubscribe(c, h, raw)
	case "unsubscribe":
		handleUnsubscribe(c, h, raw)
	case "publish":
		handlePublish(c, h, raw)
	case "ping":
		handlePing(c)
	}
}

func handleSubscribe(c *hub.Client, h *hub.Hub, raw []byte) {
	var req pkg.SubscribeRequest
	if err := parseAndValidate(raw, &req); err != nil {
		sendError(c, "subscribe", err.Error())
		return
	}
	h.AddToTopic(c, req.Topic)
	sendAck(c, "subscribe", req.Topic)
}

func handleUnsubscribe(c *hub.Client, h *hub.Hub, raw []byte) {
	var req pkg.UnsubscribeRequest
	if err := parseAndValidate(raw, &req); err != nil {
		sendError(c, "unsubscribe", err.Error())
		return
	}
	h.RemoveFromTopic(c, req.Topic)
	sendAck(c, "unsubscribe", req.Topic)
}

func handlePublish(c *hub.Client, h *hub.Hub, raw []byte) {
	var req pkg.PublishRequest
	if err := parseAndValidate(raw, &req); err != nil {
		sendError(c, "publish", err.Error())
		return
	}
	topic := h.GetOrCreateTopic(req.Topic)
	topic.Broadcast(c, pkg.NewMessage(req.Topic, req.Payload))
	sendAck(c, "publish", req.Topic)
}

func handlePing(c *hub.Client) {
	pong, _ := json.Marshal(pkg.PongResponse{Type: "pong"})
	c.Mu.Lock()
	c.Conn.Write(context.Background(), websocket.MessageText, pong)
	c.Mu.Unlock()
}

func parseAndValidate(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("invalid json")
	}
	if err := pkg.Validate.Struct(v); err != nil {
		return fmt.Errorf("%s", validationMsg(err))
	}
	return nil
}

func validationMsg(err error) string {
	if ves, ok := err.(validator.ValidationErrors); ok {
		msgs := make([]string, 0, len(ves))
		for _, ve := range ves {
			msgs = append(msgs, fmt.Sprintf("%s: %s", ve.Field(), ve.Tag()))
		}
		return strings.Join(msgs, ", ")
	}
	return err.Error()
}

func sendAck(c *hub.Client, action, topic string) {
	data := pkg.NewAck(action, topic)
	select {
	case c.Send <- data:
	default:
	}
}

func sendError(c *hub.Client, action, message string) {
	data := pkg.NewError(action, message)
	select {
	case c.Send <- data:
	default:
	}
}
