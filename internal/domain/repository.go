package domain

import (
	"context"

	"github.com/google/uuid"
)

// JobRepository defines the storage operations for jobs.
type JobRepository interface {
	Create(ctx context.Context, job *Job) error
	GetByID(ctx context.Context, id uuid.UUID) (*Job, error)
	List(ctx context.Context, status string, limit int, offset int) ([]*Job, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status JobStatus) error
	UpdateAfterProcessing(ctx context.Context, job *Job) error
}
