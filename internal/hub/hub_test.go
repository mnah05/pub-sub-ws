package hub

import (
	"sync"
	"testing"
)

func newTestClient(id string) *Client {
	return &Client{
		Id:     id,
		Send:   make(chan []byte, 256),
		Topics: make(map[string]*Topic),
	}
}

func TestNewHub(t *testing.T) {
	h := NewHub()
	if h == nil {
		t.Fatal("expected non-nil hub")
	}
	if len(h.clients) != 0 {
		t.Fatalf("expected 0 clients, got %d", len(h.clients))
	}
	if len(h.topics) != 0 {
		t.Fatalf("expected 0 topics, got %d", len(h.topics))
	}
}

func TestAddAndRemoveClient(t *testing.T) {
	h := NewHub()
	c := newTestClient("1")

	h.AddClient(c)
	if len(h.ListClients()) != 1 {
		t.Fatalf("expected 1 client, got %d", len(h.ListClients()))
	}

	h.RemoveClient(c)
	if len(h.ListClients()) != 0 {
		t.Fatalf("expected 0 clients after removal, got %d", len(h.ListClients()))
	}
}

func TestAddToTopic(t *testing.T) {
	h := NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	h.AddToTopic(c, "news")

	topics := h.ListTopics()
	if len(topics) != 1 || topics[0] != "news" {
		t.Fatalf("expected [news], got %v", topics)
	}

	if _, ok := c.Topics["news"]; !ok {
		t.Fatal("client should be subscribed to news")
	}
}

func TestRemoveFromTopic(t *testing.T) {
	h := NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	h.AddToTopic(c, "news")
	h.RemoveFromTopic(c, "news")

	if _, ok := c.Topics["news"]; ok {
		t.Fatal("client should not be subscribed to news after unsubscribe")
	}

	topics := h.ListTopics()
	if len(topics) != 0 {
		t.Fatalf("expected empty topics after unsubscribe, got %v", topics)
	}
}

func TestRemoveClientCleansUpTopics(t *testing.T) {
	h := NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	h.AddToTopic(c, "news")
	h.AddToTopic(c, "sports")

	h.RemoveClient(c)

	topics := h.ListTopics()
	if len(topics) != 0 {
		t.Fatalf("expected all topics cleaned up, got %v", topics)
	}
	_, ok := <-c.Send
	if ok {
		t.Fatal("expected Send channel to be closed")
	}
}

func TestGetOrCreateTopicIdempotent(t *testing.T) {
	h := NewHub()
	t1 := h.GetOrCreateTopic("news")
	t2 := h.GetOrCreateTopic("news")
	if t1 != t2 {
		t.Fatal("expected same topic instance for same name")
	}
}

func TestBroadcastExcludesSender(t *testing.T) {
	h := NewHub()
	sender := newTestClient("1")
	receiver := newTestClient("2")
	h.AddClient(sender)
	h.AddClient(receiver)

	h.AddToTopic(sender, "room")
	h.AddToTopic(receiver, "room")

	topic := h.GetOrCreateTopic("room")
	topic.Broadcast(sender, []byte("hello"))

	select {
	case msg := <-receiver.Send:
		if string(msg) != "hello" {
			t.Fatalf("expected 'hello', got '%s'", string(msg))
		}
	default:
		t.Fatal("expected receiver to get the message")
	}

	select {
	case <-sender.Send:
		t.Fatal("sender should not receive its own message")
	default:
	}
}

func TestBroadcastNoSubscribers(t *testing.T) {
	h := NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	topic := h.GetOrCreateTopic("empty")
	topic.Broadcast(c, []byte("hello"))
}

func TestDuplicateSubscribe(t *testing.T) {
	h := NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	h.AddToTopic(c, "news")
	h.AddToTopic(c, "news")

	topic := h.GetOrCreateTopic("news")
	topic.mu.RLock()
	count := len(topic.subscribers)
	topic.mu.RUnlock()

	if count != 1 {
		t.Fatalf("expected 1 subscriber after duplicate subscribe, got %d", count)
	}
}

func TestConcurrentAddClients(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c := newTestClient(id)
			h.AddClient(c)
		}(string(rune('a' + i%26)) + string(rune('0'+i%10)))
	}
	wg.Wait()

	clients := h.ListClients()
	if len(clients) != 100 {
		t.Fatalf("expected 100 clients, got %d", len(clients))
	}
}

func TestConcurrentPublish(t *testing.T) {
	h := NewHub()
	sender := newTestClient("sender")
	receiver := newTestClient("receiver")
	h.AddClient(sender)
	h.AddClient(receiver)

	h.AddToTopic(sender, "room")
	h.AddToTopic(receiver, "room")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			topic := h.GetOrCreateTopic("room")
			topic.Broadcast(sender, []byte("msg"))
		}()
	}
	wg.Wait()

	count := 0
	for {
		select {
		case <-receiver.Send:
			count++
		default:
			goto done
		}
	}
done:
	if count != 50 {
		t.Fatalf("expected 50 messages, got %d", count)
	}
}

func TestCleanupEmptyTopic(t *testing.T) {
	h := NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	h.AddToTopic(c, "news")
	h.RemoveFromTopic(c, "news")

	topics := h.ListTopics()
	if len(topics) != 0 {
		t.Fatalf("expected empty topics, got %v", topics)
	}
}

func TestListClientsAndTopics(t *testing.T) {
	h := NewHub()

	c1 := newTestClient("1")
	c2 := newTestClient("2")
	h.AddClient(c1)
	h.AddClient(c2)

	h.AddToTopic(c1, "news")
	h.AddToTopic(c1, "sports")

	clients := h.ListClients()
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}

	topics := h.ListTopics()
	if len(topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(topics))
	}
}

func TestRemoveFromNonexistentTopic(t *testing.T) {
	h := NewHub()
	c := newTestClient("1")
	h.AddClient(c)

	h.RemoveFromTopic(c, "nonexistent")

	if len(h.ListTopics()) != 0 {
		t.Fatal("should be no-op for nonexistent topic")
	}
}

func TestBroadcastToMultipleReceivers(t *testing.T) {
	h := NewHub()
	sender := newTestClient("sender")
	r1 := newTestClient("r1")
	r2 := newTestClient("r2")
	r3 := newTestClient("r3")
	h.AddClient(sender)
	h.AddClient(r1)
	h.AddClient(r2)
	h.AddClient(r3)

	h.AddToTopic(sender, "room")
	h.AddToTopic(r1, "room")
	h.AddToTopic(r2, "room")
	h.AddToTopic(r3, "room")

	topic := h.GetOrCreateTopic("room")
	topic.Broadcast(sender, []byte("hello"))

	for _, r := range []*Client{r1, r2, r3} {
		select {
		case msg := <-r.Send:
			if string(msg) != "hello" {
				t.Fatalf("expected 'hello', got '%s'", string(msg))
			}
		default:
			t.Fatal("expected receiver to get the message")
		}
	}
}