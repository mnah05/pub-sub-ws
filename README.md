# WebSocket Pub/Sub Broker

A lightweight, real-time message broker built on Go WebSockets. Topics are fully dynamic — created implicitly on first subscribe/publish and destroyed automatically when the last subscriber leaves. Fire-and-forget delivery, no external dependencies.

## Quick Start

```bash
go run cmd/main.go
```

Connect with `websocat` or `wscat`:

```bash
websocat ws://localhost:8080/ws
```

## Messages

### Client → Server

| Action | Required Fields | Notes |
|--------|----------------|-------|
| `subscribe` | `topic` (string, max 256) | Idempotent if already subscribed |
| `unsubscribe` | `topic` (string) | Silently ignored if not subscribed |
| `publish` | `topic`, `payload` (max 64 KB) | Message dropped if no subscribers |
| `ping` | — | Keepalive |

Example:

```json
{"action":"subscribe","topic":"news"}
{"action":"publish","topic":"news","payload":{"headline":"Go 1.26"}}
{"action":"unsubscribe","topic":"news"}
```

### Server → Client

| Type | Trigger |
|------|---------|
| `system` | On connect (includes `client_id`) |
| `ack` | After every successful action |
| `message` | When a subscribed topic receives a publish |
| `pong` | Response to `ping` |
| `error` | On malformed input, unknown action, or validation failure |

Example:

```json
{"type":"ack","action":"subscribe","topic":"news"}
{"type":"message","topic":"news","payload":{"headline":"Go 1.26"},"timestamp":"2026-04-21T11:23:00Z"}
```

## REST Endpoints

```bash
curl http://localhost:8080/topics   # active topicscurl http://localhost:8080/clients  # connected clients
```

## Limits

| Limit | Value |
|-------|-------|
| Topic name length | 256 characters |
| Message payload | 64 KB (65 536 bytes) |
| Heartbeat | 30s ping / 60s timeout (deferred) |

## Architecture

```
cmd/main.go                — Entry point
internal/
  hub/hub.go               — Topic/Client structs, lifecycle, broadcast
  client/client.go         — ReadLoop, WriteLoop, Cleanup
  handler/handler.go       — Dispatch, validation, action handlers
  server/server.go         — HTTP server, WebSocket upgrade, REST endpoints
pkg/
  response.server.go       — Response types
  response.client.go       — Request types + validator
```

Each connection spawns ReadLoop + WriteLoop sharing a `context.WithCancel`. On disconnect, cleanup removes the client from all topics and destroys empty topics automatically.

## Testing

```bash
go test ./... -v
```

Tests cover hub operations, handler dispatch, end-to-end subscribe/publish/concurrency, malformed input, admin endpoints, and dead connection cleanup.

## Evaluation Checklist

- SUBSCRIBE / UNSUBSCRIBE with acks ✅
- PUBLISH delivers to all topic subscribers ✅
- Topics created implicitly, destroyed when empty ✅
- Thread-safe under concurrent load ✅
- Proper cleanup on disconnect (no leaks) ✅
- Resilient to malformed input and size limits ✅
- Message ordering preserved per topic ✅
- Heartbeat ping/pong with timeout cleanup ⏸ (deferred)
