 # WebSocket Pub/Sub Broker

A lightweight, real-time message broker built on WebSockets using Go (`coder/websocket`). Topics are fully dynamic — created implicitly on first subscribe/publish and destroyed automatically when the last subscriber leaves. No Redis, no external dependencies, just clean concurrency.

---

## Core Concepts

**Topic** — A named channel (e.g., `chat.room-42`, `alerts.critical`). Clients subscribe to topics to receive messages. Topics are created automatically when the first client subscribes or a message is published. Topics are destroyed automatically when the last subscriber leaves.

**Client** — Any WebSocket connection. Each client receives a unique ID on connection. A single client can subscribe to multiple topics simultaneously.

**Message** — A JSON payload published to a topic. The broker delivers it to all current subscribers of that topic. Messages are fire-and-forget — no persistence, no replay for late subscribers.

---

## Client → Server Messages

All communication uses JSON frames sent over the WebSocket connection.

### SUBSCRIBE

Join a topic to start receiving messages.

```json
{
  "action": "subscribe",
  "topic": "news.sports"
}
```

**Validation:**
- `topic` is required, non-empty string
- `topic` maximum length: **256 characters**
- Duplicate subscribes are idempotent (no error, ack still sent)

---

### UNSUBSCRIBE

Leave a topic to stop receiving messages.

```json
{
  "action": "unsubscribe",
  "topic": "news.sports"
}
```

**Validation:**
- `topic` is required, non-empty string
- Silently ignored if not subscribed

---

### PUBLISH

Send a message to all subscribers of a topic.

```json
{
  "action": "publish",
  "topic": "news.sports",
  "payload": {
    "score": "2-1",
    "minute": 67
  }
}
```

**Validation:**
- `topic` is required, non-empty string, max 256 characters
- `payload` is required, max **65 536 bytes** (64 KB)
- If topic has no subscribers, message is dropped (topic still created implicitly)

---

### LIST_TOPICS

Request a list of all active topics.

```json
{
  "action": "list_topics"
}
```

**Response:** Server replies with active topic names. A topic is active if it has at least one subscriber.

---

### PING

Keepalive heartbeat to prevent connection timeout.

```json
{
  "action": "ping"
}
```

**Response:** Server replies with `pong`.

---

## Server → Client Messages

### ACK

Confirms a client action succeeded.

```json
{
  "type": "ack",
  "action": "subscribe",
  "topic": "news.sports"
}
```

Sent after every successful `subscribe`, `unsubscribe`, and `publish`.

---

### MESSAGE

A message published to a topic the client is subscribed to.

```json
{
  "type": "message",
  "topic": "news.sports",
  "payload": {
    "score": "2-1",
    "minute": 67
  },
  "timestamp": "2026-04-21T11:23:00Z"
}
```

- `timestamp` is ISO 8601 UTC
- Delivered to all subscribers except the publisher (optional: include self, document your choice)

---

### PONG

Heartbeat response.

```json
{
  "type": "pong"
}
```

---

### TOPICS_LIST

Response to `list_topics`.

```json
{
  "type": "topics_list",
  "topics": ["news.sports", "chat.general", "alerts.critical"],
  "count": 3
}
```

---

### ERROR

Action failed. Connection stays open.

```json
{
  "type": "error",
  "action": "publish",
  "message": "payload exceeds maximum size of 64KB"
}
```

---

### SYSTEM

Sent once on connection establishment.

```json
{
  "type": "system",
  "client_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "message": "connected to broker"
}
```

---

## Connection Lifecycle

1. **Connect** — Client opens WebSocket. Server assigns UUID, sends `system` message.
2. **Heartbeat** — Client must send `ping` every **30 seconds**. Server responds `pong`. If no frame received for **60 seconds**, server closes connection.
3. **Subscribe** — Client joins topics of interest.
4. **Interact** — Client publishes and receives messages.
5. **Disconnect** — Client closes WebSocket or times out. Server removes client from all topics and destroys any topics that become empty.

---

## Limits & Constraints

| Limit | Value | Rationale |
|-------|-------|-----------|
| Max topic name length | 256 characters | Prevents abuse, keeps lookups fast |
| Max message payload | 64 KB (65 536 bytes) | Prevents memory pressure from giant frames |
| Max total frame size | 64 KB + overhead | Hard cutoff — reject with error |
| Heartbeat interval | 30 seconds | Keep NATs and load balancers happy |
| Heartbeat timeout | 60 seconds | Detect dead connections promptly |

---

## Required Behaviors

### Dynamic Topic Management
- Topics are created on first `subscribe` or `publish`
- Topics are destroyed when the last subscriber unsubscribes or disconnects
- No manual topic creation or deletion actions exist

### Thread Safety
- Multiple clients may subscribe, unsubscribe, and publish concurrently
- The broker must not crash or corrupt state under concurrent load

### Graceful Disconnect Handling
- When a client disconnects (clean close, timeout, or network failure), remove them from all subscribed topics
- Clean up any topics that become empty after removal
- No memory leaks from stale connections or abandoned topics

### Error Resilience
- Malformed JSON → send `error` response, keep connection open
- Unknown action → send `error` response, keep connection open
- Missing required fields → send `error` response, keep connection open
- Oversized payload → send `error` response, do not broadcast

### Message Ordering
- Messages within a single topic must be delivered to subscribers in the order they were published
- Ordering across different topics is not guaranteed

---

## Out of Scope

These are intentionally excluded to keep the project focused:

- Message persistence or replay
- Authentication or authorization
- Wildcard topic matching
- Quality of Service (QoS) levels
- Rate limiting or backpressure
- Metrics or observability endpoints
- Multi-node clustering
- Graceful shutdown with drain

---

## Evaluation Checklist

| Criteria | Status |
|----------|--------|
| SUBSCRIBE / UNSUBSCRIBE work with acks | Done |
| PUBLISH delivers to all topic subscribers | Done |
| LIST_TOPICS returns only active topics | Done |
| Topics created implicitly, destroyed when empty | Done |
| Heartbeat ping/pong with timeout cleanup | Deferred |
| Connection info sent on connect | Done |
| Thread-safe under concurrent load | Done |
| Proper cleanup on disconnect (no leaks) | Done |
| Resilient to malformed input | Done |
| Respects size limits (topic name, payload) | Done |
| Message ordering preserved per topic | Done |

---

## Architecture

```
cmd/main.go                — Entry point
internal/
  hub/hub.go               — Topic/Client structs, topic lifecycle, broadcast
  client/client.go         — ReadLoop, WriteLoop, Cleanup, connection factory
  handler/handler.go       — Message dispatch, validation, action handlers
  server/server.go         — HTTP server, WebSocket upgrade, /topics /clients endpoints
pkg/
  response.server.go       — Response types (Ack, Message, Error, etc.)
  response.client.go       — Request types with validator tags
```

```
┌─────────────┐      ┌─────────────────────────┐      ┌─────────────┐
│  Client A   │◄────►│                         │◄────►│  Client B   │
│ (sub: news) │      │      WebSocket Broker   │      │(sub: news)  │
└─────────────┘      │                         │      └─────────────┘
                     │  ┌─────────────────┐    │
                     │  │   Topic Map     │    │
                     │  │  news → {A, B}  │    │
                     │  │  chat → {C}     │    │
                     │  └─────────────────┘    │
                     │                         │
                     │  ┌─────────────────┐    │
                     │  │  Client Registry│    │
                     │  │  A → {news}     │    │
                     │  │  B → {news}     │    │
                     │  │  C → {chat}     │    │
                     │  └─────────────────┘    │
                     │                         │
                     └─────────────────────────┘
```

Each connection spawns two goroutines (ReadLoop + WriteLoop) sharing a `context.WithCancel`. When either detects a failure, it cancels the context, stopping both loops and triggering cleanup.

---

## Running & Testing

```bash
# Start the server
go run cmd/main.go

# Connect with websocat
websocat ws://localhost:8080/ws

# Or with wscat
wscat -c ws://localhost:8080/ws
```

### Example session

```json
→ {"action":"subscribe","topic":"news"}
← {"type":"ack","action":"subscribe","topic":"news"}

→ {"action":"publish","topic":"news","payload":{"headline":"Go 1.26 released"}}
← {"type":"ack","action":"publish","topic":"news"}

→ {"action":"ping"}
← {"type":"pong"}

→ {"action":"unsubscribe","topic":"news"}
← {"type":"ack","action":"unsubscribe","topic":"news"}
```

### REST endpoints

```bash
# List active topics
curl http://localhost:8080/topics

# List connected clients
curl http://localhost:8080/clients
```

### Run tests

```bash
go test ./... -v
```

---

## Submission Requirements

- Working server with WebSocket endpoint
- Brief README explaining how to run and test
- A simple test script or `curl`/`websocat` commands demonstrating all features
- No external message brokers (Redis, NATS, etc.) — pure WebSocket implementation
