package handler

import (
	"encoding/json"
	"testing"

	"pubsub/internal/hub"
	"pubsub/pkg"
)

func newTestClient(id string) *hub.Client {
	return &hub.Client{
		Id:     id,
		Send:   make(chan []byte, 256),
		Topics: make(map[string]*hub.Topic),
	}
}

func TestDispatchInvalidJSON(t *testing.T) {
	h := hub.NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	Dispatch(c, h, []byte("not json"))

	select {
	case msg := <-c.Send:
		var errResp pkg.ErrorResponse
		if err := json.Unmarshal(msg, &errResp); err != nil {
			t.Fatalf("expected error response, got %s", string(msg))
		}
		if errResp.Type != "error" {
			t.Fatalf("expected type 'error', got '%s'", errResp.Type)
		}
	default:
		t.Fatal("expected error response on Send channel")
	}
}

func TestDispatchUnknownAction(t *testing.T) {
	h := hub.NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	raw, _ := json.Marshal(map[string]string{"action": "unknown"})
	Dispatch(c, h, raw)

	select {
	case msg := <-c.Send:
		var errResp pkg.ErrorResponse
		if err := json.Unmarshal(msg, &errResp); err != nil {
			t.Fatalf("expected error response, got %s", string(msg))
		}
		if errResp.Type != "error" {
			t.Fatalf("expected error type, got '%s'", errResp.Type)
		}
	default:
		t.Fatal("expected error response")
	}
}

func TestHandleSubscribe(t *testing.T) {
	h := hub.NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	raw, _ := json.Marshal(pkg.SubscribeRequest{Action: "subscribe", Topic: "news"})
	Dispatch(c, h, raw)

	select {
	case msg := <-c.Send:
		var ack pkg.AckResponse
		if err := json.Unmarshal(msg, &ack); err != nil {
			t.Fatalf("expected ack response, got %s", string(msg))
		}
		if ack.Type != "ack" || ack.Action != "subscribe" || ack.Topic != "news" {
			t.Fatalf("unexpected ack: %+v", ack)
		}
	default:
		t.Fatal("expected ack on Send channel")
	}

	if _, ok := c.Topics["news"]; !ok {
		t.Fatal("client should be subscribed to 'news'")
	}
}

func TestHandleSubscribeEmptyTopic(t *testing.T) {
	h := hub.NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	raw, _ := json.Marshal(map[string]string{"action": "subscribe", "topic": ""})
	Dispatch(c, h, raw)

	select {
	case msg := <-c.Send:
		var errResp pkg.ErrorResponse
		json.Unmarshal(msg, &errResp)
		if errResp.Type != "error" {
			t.Fatalf("expected error, got %+v", errResp)
		}
	default:
		t.Fatal("expected error response")
	}
}

func TestHandleUnsubscribe(t *testing.T) {
	h := hub.NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	h.AddToTopic(c, "news")

	raw, _ := json.Marshal(pkg.UnsubscribeRequest{Action: "unsubscribe", Topic: "news"})
	Dispatch(c, h, raw)

	select {
	case msg := <-c.Send:
		var ack pkg.AckResponse
		json.Unmarshal(msg, &ack)
		if ack.Action != "unsubscribe" || ack.Topic != "news" {
			t.Fatalf("unexpected ack: %+v", ack)
		}
	default:
		t.Fatal("expected ack")
	}

	if _, ok := c.Topics["news"]; ok {
		t.Fatal("client should not be subscribed to 'news' after unsubscribe")
	}
}

func TestHandlePublish(t *testing.T) {
	h := hub.NewHub()
	sender := newTestClient("1")
	receiver := newTestClient("2")
	h.AddClient(sender)
	h.AddClient(receiver)

	h.AddToTopic(sender, "room")
	h.AddToTopic(receiver, "room")

	raw, _ := json.Marshal(pkg.PublishRequest{
		Action:  "publish",
		Topic:   "room",
		Payload: json.RawMessage(`"hello"`),
	})
	Dispatch(sender, h, raw)

	select {
	case msg := <-sender.Send:
		var ack pkg.AckResponse
		json.Unmarshal(msg, &ack)
		if ack.Action != "publish" {
			t.Fatalf("expected publish ack, got %+v", ack)
		}
	default:
		t.Fatal("sender should receive ack")
	}

	select {
	case msg := <-receiver.Send:
		var m pkg.MessageResponse
		json.Unmarshal(msg, &m)
		if m.Type != "message" || m.Topic != "room" {
			t.Fatalf("unexpected message: %+v", m)
		}
	default:
		t.Fatal("receiver should get published message")
	}
}

func TestHandlePublishEmptyTopic(t *testing.T) {
	h := hub.NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	raw, _ := json.Marshal(map[string]string{"action": "publish", "topic": ""})
	Dispatch(c, h, raw)

	select {
	case msg := <-c.Send:
		var errResp pkg.ErrorResponse
		json.Unmarshal(msg, &errResp)
		if errResp.Type != "error" {
			t.Fatalf("expected error, got %+v", errResp)
		}
	default:
		t.Fatal("expected error response")
	}
}

func TestDuplicateSubscribe(t *testing.T) {
	h := hub.NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	for i := 0; i < 3; i++ {
		raw, _ := json.Marshal(pkg.SubscribeRequest{Action: "subscribe", Topic: "news"})
		Dispatch(c, h, raw)
		<-c.Send
	}

	topics := h.ListTopics()
	if len(topics) != 1 || topics[0] != "news" {
		t.Fatalf("expected exactly [news], got %v", topics)
	}
}
