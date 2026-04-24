package pkg

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

type AckResponse struct {
	Type   string `json:"type"`
	Action string `json:"action"`
	Topic  string `json:"topic"`
}

type MessageResponse struct {
	Type      string          `json:"type"`
	Topic     string          `json:"topic"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp string          `json:"timestamp"`
}

type PongResponse struct {
	Type string `json:"type"`
}

type TopicsListResponse struct {
	Type   string   `json:"type"`
	Topics []string `json:"topics"`
	Count  int      `json:"count"`
}

type ClientsListResponse struct {
	Type     string   `json:"type"`
	Clients []string `json:"clients"`
	Count    int      `json:"count"`
}

type ErrorResponse struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	Message string `json:"message"`
}

type SystemResponse struct {
	Type     string `json:"type"`
	ClientID string `json:"client_id"`
	Message  string `json:"message"`
}

func WriteJSON(conn *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.Write(context.Background(), websocket.MessageText, data)
}

func NewMessage(topic string, payload json.RawMessage) []byte {
	resp := MessageResponse{
		Type:      "message",
		Topic:     topic,
		Payload:   payload,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(resp)
	return data
}

func NewAck(action, topic string) []byte {
	data, _ := json.Marshal(AckResponse{
		Type:   "ack",
		Action: action,
		Topic:  topic,
	})
	return data
}

func NewError(action, message string) []byte {
	data, _ := json.Marshal(ErrorResponse{
		Type:    "error",
		Action:  action,
		Message: message,
	})
	return data
}
