// Client → Server request types

package pkg

import (
	"encoding/json"

	"github.com/go-playground/validator/v10"
)

var Validate = validator.New(validator.WithRequiredStructEnabled())

// SubscribeRequest requests subscription to a topic.
type SubscribeRequest struct {
	Action string `json:"action" validate:"required,eq=subscribe"`
	Topic  string `json:"topic" validate:"required,min=1,max=256"`
}

// UnsubscribeRequest unsubscribes from a topic.
type UnsubscribeRequest struct {
	Action string `json:"action" validate:"required,eq=unsubscribe"`
	Topic  string `json:"topic" validate:"required,min=1,max=256"`
}

// PublishRequest broadcasts a message to topic subscribers.
type PublishRequest struct {
	Action  string          `json:"action" validate:"required,eq=publish"`
	Topic   string          `json:"topic" validate:"required,min=1,max=256"`
	Payload json.RawMessage `json:"payload" validate:"required,max=65536"`
}

// ListTopicsRequest requests the list of active topics.
type ListTopicsRequest struct {
	Action string `json:"action" validate:"required,eq=list_topics"`
}

// PingRequest is a keepalive heartbeat.
type PingRequest struct {
	Action string `json:"action" validate:"required,eq=ping"`
}
