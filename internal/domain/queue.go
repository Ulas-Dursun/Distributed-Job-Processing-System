package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// JobQueue defines the queue operations for scheduling and dispatching jobs.
type JobQueue interface {
	Push(ctx context.Context, jobID uuid.UUID) error
	Pop(ctx context.Context, timeout time.Duration) (uuid.UUID, error)
	Len(ctx context.Context) (int64, error)
}
