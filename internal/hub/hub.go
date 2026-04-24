package hub

import (
	"sync"

	"github.com/coder/websocket"
)

// Hub manages topics and broadcasts messages to subscribers.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
	topics  map[string]*Topic
}

// Client represents a connected WebSocket client.
type Client struct {
	Mu     sync.Mutex
	Id     string
	Conn   *websocket.Conn
	Send   chan []byte
	Topics map[string]*Topic // subscribed topics
}

// Topic represents a pub/sub topic.
type Topic struct {
	mu          sync.RWMutex
	Name        string
	subscribers map[*Client]bool
}

// Broadcast sends data to all subscribers except the sender.
func (t *Topic) Broadcast(sender *Client, data []byte) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for sub := range t.subscribers {
		if sub == sender {
			continue
		}
		select {
		case sub.Send <- data:
		default:
		}
	}
}

func (h *Hub) ListClients() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	clients := make([]string, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c.Id)
	}
	return clients
}

func (h *Hub) ListTopics() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	topics := make([]string, 0, len(h.topics))
	for name := range h.topics {
		topics = append(topics, name)
	}
	return topics
}

// NewHub creates and initializes a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]bool),
		topics:  make(map[string]*Topic),
	}
}

// GetOrCreateTopic returns an existing topic or creates a new one.
func (h *Hub) GetOrCreateTopic(name string) *Topic {
	h.mu.Lock()
	defer h.mu.Unlock()

	if t, ok := h.topics[name]; ok {
		return t
	}

	t := &Topic{
		Name:        name,
		subscribers: make(map[*Client]bool),
	}
	h.topics[name] = t
	return t
}

// AddToTopic adds a client as a subscriber to a topic.
func (h *Hub) AddToTopic(client *Client, topicName string) {
	t := h.GetOrCreateTopic(topicName)

	t.mu.Lock()
	t.subscribers[client] = true
	t.mu.Unlock()

	client.Mu.Lock()
	client.Topics[topicName] = t
	client.Mu.Unlock()
}

// RemoveFromTopic removes a client from a topic's subscribers.
// If the topic becomes empty, it is removed from the hub.
func (h *Hub) RemoveFromTopic(client *Client, topicName string) {
	h.mu.RLock()
	t, ok := h.topics[topicName]
	h.mu.RUnlock()
	if !ok {
		return
	}

	t.mu.Lock()
	delete(t.subscribers, client)
	empty := len(t.subscribers) == 0
	t.mu.Unlock()

	client.Mu.Lock()
	delete(client.Topics, topicName)
	client.Mu.Unlock()

	if empty {
		h.CleanupEmptyTopic(t)
	}
}

// AddClient registers a client in the hub.
func (h *Hub) AddClient(c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
}

// RemoveClient removes a client from the hub and unsubscribes it from all topics.
func (h *Hub) RemoveClient(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()

	c.Mu.Lock()
	topics := make([]*Topic, 0, len(c.Topics))
	for _, t := range c.Topics {
		topics = append(topics, t)
	}
	c.Mu.Unlock()

	for _, t := range topics {
		h.RemoveFromTopic(c, t.Name)
	}

	close(c.Send)
}

// CleanupEmptyTopic removes the topic from the hub if it has no subscribers.
func (h *Hub) CleanupEmptyTopic(t *Topic) {
	h.mu.Lock()
	defer h.mu.Unlock()

	t.mu.RLock()
	empty := len(t.subscribers) == 0
	t.mu.RUnlock()

	if empty {
		delete(h.topics, t.Name)
	}
}
