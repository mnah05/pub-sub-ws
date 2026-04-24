package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"pubsub/internal/hub"
	"pubsub/pkg"
)

func startTestServer(t *testing.T) (*httptest.Server, *hub.Hub) {
	t.Helper()
	h := hub.NewHub()
	s := &Server{Hub: h}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.wsHandler)
	mux.HandleFunc("/topics", s.topicHandler)
	mux.HandleFunc("/clients", s.clientHandler)

	ts := httptest.NewServer(mux)
	return ts, h
}

func wsURL(ts *httptest.Server) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
}

func connect(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(ts), nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	return conn
}

func readMsg(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	return msg
}

func writeMsg(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = conn.Write(ctx, websocket.MessageText, data)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
}

func skipSystemMsg(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	msg := readMsg(t, conn)
	var sys pkg.SystemResponse
	if err := json.Unmarshal(msg, &sys); err != nil || sys.Type != "system" {
		t.Fatalf("expected system message, got %s", string(msg))
	}
}

func TestE2ESubscribeAndPublish(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	publisher := connect(t, ts)
	defer publisher.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, publisher)

	subscriber := connect(t, ts)
	defer subscriber.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, subscriber)

	writeMsg(t, subscriber, pkg.SubscribeRequest{Action: "subscribe", Topic: "room"})
	readMsg(t, subscriber)

	writeMsg(t, publisher, pkg.SubscribeRequest{Action: "subscribe", Topic: "room"})
	readMsg(t, publisher)

	writeMsg(t, publisher, pkg.PublishRequest{
		Action:  "publish",
		Topic:   "room",
		Payload: json.RawMessage(`"hello"`),
	})
	readMsg(t, publisher)

	msg := readMsg(t, subscriber)
	var received pkg.MessageResponse
	if err := json.Unmarshal(msg, &received); err != nil {
		t.Fatalf("expected message response, got %s", string(msg))
	}
	if received.Type != "message" || received.Topic != "room" {
		t.Fatalf("unexpected message: %+v", received)
	}
}

func TestE2EDuplicateSubscribe(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	conn := connect(t, ts)
	defer conn.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, conn)

	writeMsg(t, conn, pkg.SubscribeRequest{Action: "subscribe", Topic: "news"})
	readMsg(t, conn)

	writeMsg(t, conn, pkg.SubscribeRequest{Action: "subscribe", Topic: "news"})
	readMsg(t, conn)

	resp, err := http.Get(ts.URL + "/topics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var topicsResp pkg.TopicsListResponse
	json.NewDecoder(resp.Body).Decode(&topicsResp)
	if topicsResp.Count != 1 {
		t.Fatalf("expected 1 topic after duplicate subscribe, got %d", topicsResp.Count)
	}
}

func TestE2EConcurrentPublish(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	receiver := connect(t, ts)
	defer receiver.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, receiver)

	writeMsg(t, receiver, pkg.SubscribeRequest{Action: "subscribe", Topic: "room"})
	readMsg(t, receiver)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := connect(t, ts)
			defer c.Close(websocket.StatusNormalClosure, "")
			skipSystemMsg(t, c)
			writeMsg(t, c, pkg.PublishRequest{
				Action:  "publish",
				Topic:   "room",
				Payload: json.RawMessage(`"hello"`),
			})
		}()
	}
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := 0
	for {
		_, _, err := receiver.Read(ctx)
		if err != nil {
			break
		}
		count++
	}
	if count != 10 {
		t.Fatalf("expected 10 messages, got %d", count)
	}
}

func TestE2EDeadConnectionCleanup(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	conn := connect(t, ts)
	skipSystemMsg(t, conn)

	resp, err := http.Get(ts.URL + "/clients")
	if err != nil {
		t.Fatal(err)
	}
	var before pkg.ClientsListResponse
	json.NewDecoder(resp.Body).Decode(&before)
	resp.Body.Close()
	if before.Count < 1 {
		t.Fatalf("expected at least 1 client, got %d", before.Count)
	}

	conn.Close(websocket.StatusPolicyViolation, "abrupt close")

	time.Sleep(500 * time.Millisecond)

	resp, err = http.Get(ts.URL + "/clients")
	if err != nil {
		t.Fatal(err)
	}
	var after pkg.ClientsListResponse
	json.NewDecoder(resp.Body).Decode(&after)
	resp.Body.Close()
	if after.Count >= before.Count {
		t.Fatalf("expected client count to decrease after disconnect, before=%d after=%d", before.Count, after.Count)
	}
}

func TestE2EUnsubscribe(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	conn := connect(t, ts)
	defer conn.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, conn)

	writeMsg(t, conn, pkg.SubscribeRequest{Action: "subscribe", Topic: "news"})
	readMsg(t, conn)

	writeMsg(t, conn, pkg.UnsubscribeRequest{Action: "unsubscribe", Topic: "news"})
	readMsg(t, conn)

	resp, err := http.Get(ts.URL + "/topics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var topicsResp pkg.TopicsListResponse
	json.NewDecoder(resp.Body).Decode(&topicsResp)
	if topicsResp.Count != 0 {
		t.Fatalf("expected 0 topics after unsubscribe, got %d", topicsResp.Count)
	}
}

func TestE2EPingPong(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	conn := connect(t, ts)
	defer conn.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, conn)

	writeMsg(t, conn, pkg.PingRequest{Action: "ping"})
	msg := readMsg(t, conn)

	var pong pkg.PongResponse
	if err := json.Unmarshal(msg, &pong); err != nil {
		t.Fatalf("expected pong, got %s", string(msg))
	}
	if pong.Type != "pong" {
		t.Fatalf("expected type 'pong', got '%s'", pong.Type)
	}
}

func TestE2EInvalidJSON(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	conn := connect(t, ts)
	defer conn.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn.Write(ctx, websocket.MessageText, []byte("not json"))

	msg := readMsg(t, conn)
	var errResp pkg.ErrorResponse
	json.Unmarshal(msg, &errResp)
	if errResp.Type != "error" {
		t.Fatalf("expected error response, got %s", string(msg))
	}
}

func TestE2ETopicsEndpoint(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	conn := connect(t, ts)
	defer conn.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, conn)

	writeMsg(t, conn, pkg.SubscribeRequest{Action: "subscribe", Topic: "news"})
	readMsg(t, conn)

	writeMsg(t, conn, pkg.SubscribeRequest{Action: "subscribe", Topic: "sports"})
	readMsg(t, conn)

	resp, err := http.Get(ts.URL + "/topics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var topicsResp pkg.TopicsListResponse
	json.NewDecoder(resp.Body).Decode(&topicsResp)
	if topicsResp.Count != 2 {
		t.Fatalf("expected 2 topics, got %d", topicsResp.Count)
	}
}

func TestE2EClientsEndpoint(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	c1 := connect(t, ts)
	defer c1.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, c1)

	c2 := connect(t, ts)
	defer c2.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, c2)

	resp, err := http.Get(ts.URL + "/clients")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var clientsResp pkg.ClientsListResponse
	json.NewDecoder(resp.Body).Decode(&clientsResp)
	if clientsResp.Count != 2 {
		t.Fatalf("expected 2 clients, got %d", clientsResp.Count)
	}
}

func TestE2EPublisherDoesNotReceiveOwnMessage(t *testing.T) {
	ts, _ := startTestServer(t)
	defer ts.Close()

	pub := connect(t, ts)
	defer pub.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, pub)

	sub := connect(t, ts)
	defer sub.Close(websocket.StatusNormalClosure, "")
	skipSystemMsg(t, sub)

	writeMsg(t, sub, pkg.SubscribeRequest{Action: "subscribe", Topic: "room"})
	readMsg(t, sub)

	writeMsg(t, pub, pkg.SubscribeRequest{Action: "subscribe", Topic: "room"})
	readMsg(t, pub)

	writeMsg(t, pub, pkg.PublishRequest{
		Action:  "publish",
		Topic:   "room",
		Payload: json.RawMessage(`"hello"`),
	})

	ack := readMsg(t, pub)
	var ackResp pkg.AckResponse
	json.Unmarshal(ack, &ackResp)
	if ackResp.Action != "publish" {
		t.Fatalf("expected publish ack, got %+v", ackResp)
	}

	msg := readMsg(t, sub)
	var received pkg.MessageResponse
	json.Unmarshal(msg, &received)
	if received.Type != "message" {
		t.Fatalf("expected message, got %+v", received)
	}
}
