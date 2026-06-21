package tasks

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TypeSendToN8n = "webhook:n8n_send"

type SendToN8nPayload struct {
	PostID uuid.UUID `json:"post_id"`
}

func NewSendToN8nTask(postID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(SendToN8nPayload{PostID: postID})
	if err != nil {
		return nil, err
	}

	return asynq.NewTask(
		TypeSendToN8n,
		payload,
		asynq.Queue("critical"),
		asynq.MaxRetry(5),
	), nil
}
