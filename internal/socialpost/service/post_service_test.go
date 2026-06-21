package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/n8n/socialpost/internal/socialpost/model"
	"github.com/n8n/socialpost/internal/socialpost/repository"
	"github.com/n8n/socialpost/internal/worker/tasks"
)

type fakeRepo struct {
	createFn       func(context.Context, model.Post) (model.Post, error)
	updateStatusFn func(context.Context, uuid.UUID, string) error

	lastCreateInput model.Post
	createCalls     int
	updateCalls     int
	lastUpdateID    uuid.UUID
	lastUpdateValue string
}

func (f *fakeRepo) Create(ctx context.Context, p model.Post) (model.Post, error) {
	f.createCalls++
	f.lastCreateInput = p
	if f.createFn != nil {
		return f.createFn(ctx, p)
	}
	return model.Post{ID: uuid.New()}, nil
}

func (f *fakeRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	f.updateCalls++
	f.lastUpdateID = id
	f.lastUpdateValue = status
	if f.updateStatusFn != nil {
		return f.updateStatusFn(ctx, id, status)
	}
	return nil
}

type fakeEnqueuer struct {
	task  *asynq.Task
	calls int
	err   error
}

func (f *fakeEnqueuer) EnqueueContext(ctx context.Context, task *asynq.Task) (*asynq.TaskInfo, error) {
	f.calls++
	f.task = task
	if f.err != nil {
		return nil, f.err
	}
	return &asynq.TaskInfo{Queue: "critical"}, nil
}

func TestCreatePost_Success(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{
		createFn: func(ctx context.Context, p model.Post) (model.Post, error) {
			return model.Post{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111")}, nil
		},
	}
	enqueuer := &fakeEnqueuer{}
	svc := NewPostService(repo, enqueuer, slog.New(slog.NewTextHandler(new(testWriter), nil)))

	err := svc.CreatePost(context.Background(), CreatePostInput{
		Payload: json.RawMessage(`{"b":2,"a":1}`),
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, 1, enqueuer.calls)
	require.Equal(t, `{"a":1,"b":2}`, string(repo.lastCreateInput.Payload))
	require.NotEmpty(t, repo.lastCreateInput.ContentHash)
	require.Equal(t, model.StatusPending, repo.lastCreateInput.Status)
	require.Equal(t, tasks.TypeSendToN8n, enqueuer.task.Type())
}

func TestCreatePost_DuplicateIsIdempotent(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{
		createFn: func(ctx context.Context, p model.Post) (model.Post, error) {
			return model.Post{}, repository.ErrDuplicateHash
		},
	}
	enqueuer := &fakeEnqueuer{}
	svc := NewPostService(repo, enqueuer, slog.Default())

	err := svc.CreatePost(context.Background(), CreatePostInput{
		Payload: json.RawMessage(`{"a":1}`),
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Zero(t, enqueuer.calls)
}

func TestCreatePost_DeterministicHash(t *testing.T) {
	t.Parallel()

	hashFor := func(raw string) string {
		repo := &fakeRepo{
			createFn: func(ctx context.Context, p model.Post) (model.Post, error) {
				return model.Post{ID: uuid.New()}, nil
			},
		}
		svc := NewPostService(repo, &fakeEnqueuer{}, slog.Default())

		err := svc.CreatePost(context.Background(), CreatePostInput{Payload: json.RawMessage(raw)})
		require.NoError(t, err)
		require.NotEmpty(t, repo.lastCreateInput.ContentHash)
		return repo.lastCreateInput.ContentHash
	}

	require.Equal(t,
		hashFor(`{"b":2,"a":1}`),
		hashFor(`{"a":1,"b":2}`),
	)
}

func TestCreatePost_EnqueueFailureMarksFailed(t *testing.T) {
	t.Parallel()

	postID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	repo := &fakeRepo{
		createFn: func(ctx context.Context, p model.Post) (model.Post, error) {
			return model.Post{ID: postID}, nil
		},
	}
	enqueuer := &fakeEnqueuer{err: errors.New("redis unavailable")}
	svc := NewPostService(repo, enqueuer, slog.Default())

	err := svc.CreatePost(context.Background(), CreatePostInput{
		Payload: json.RawMessage(`{"a":1}`),
	})
	require.Error(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, 1, enqueuer.calls)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, postID, repo.lastUpdateID)
	require.Equal(t, model.StatusFailed, repo.lastUpdateValue)
}

type testWriter struct{}

func (w *testWriter) Write(p []byte) (int, error) { return len(p), nil }
