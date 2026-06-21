package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/n8n/socialpost/internal/socialpost/model"
	"github.com/n8n/socialpost/internal/socialpost/repository"
	"github.com/n8n/socialpost/internal/worker/tasks"
)

type TaskEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task) (*asynq.TaskInfo, error)
}

type CreatePostInput struct {
	Payload json.RawMessage
}

type PostService struct {
	repo     repository.PostRepository
	enqueuer TaskEnqueuer
	logger   *slog.Logger
}

func NewPostService(repo repository.PostRepository, enqueuer TaskEnqueuer, logger *slog.Logger) *PostService {
	return &PostService{
		repo:     repo,
		enqueuer: enqueuer,
		logger:   logger,
	}
}

func (s *PostService) CreatePost(ctx context.Context, input CreatePostInput) error {
	payload, err := canonicalizeJSON(input.Payload)
	if err != nil {
		return fmt.Errorf("service.CreatePost: canonicalize payload: %w", err)
	}

	sum := sha256.Sum256(payload)
	post := model.Post{
		ContentHash: hex.EncodeToString(sum[:]),
		Payload:     payload,
		Status:      model.StatusPending,
	}

	created, err := s.repo.Create(ctx, post)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateHash) {
			s.loggerFor(ctx).InfoContext(ctx, "duplicate social post ignored",
				"content_hash", post.ContentHash,
			)
			return nil
		}
		return fmt.Errorf("service.CreatePost: create post: %w", err)
	}

	task, err := tasks.NewSendToN8nTask(created.ID)
	if err != nil {
		return s.compensateFailedPost(ctx, created.ID, fmt.Errorf("service.CreatePost: build task: %w", err))
	}

	if _, err := s.enqueuer.EnqueueContext(ctx, task); err != nil {
		return s.compensateFailedPost(ctx, created.ID, fmt.Errorf("service.CreatePost: enqueue task: %w", err))
	}

	return nil
}

func (s *PostService) compensateFailedPost(ctx context.Context, postID uuid.UUID, cause error) error {
	if err := s.repo.UpdateStatus(ctx, postID, model.StatusFailed); err != nil {
		s.loggerFor(ctx).ErrorContext(ctx, "failed to mark social post as failed",
			"post_id", postID,
			"err", err,
			"cause", cause,
		)
	} else {
		s.loggerFor(ctx).ErrorContext(ctx, "failed to enqueue social post task",
			"post_id", postID,
			"cause", cause,
		)
	}

	return cause
}

func (s *PostService) loggerFor(ctx context.Context) *slog.Logger {
	if s != nil && s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

func canonicalizeJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return json.Marshal(nil)
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}

	return json.Marshal(decoded)
}
