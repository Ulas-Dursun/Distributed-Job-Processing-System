package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/ulasdursun/queueforge/internal/domain"
)

const (
	jobQueueKey = "queueforge:jobs"
)

// Ensure RedisQueue satisfies domain.JobQueue at compile time.
var _ domain.JobQueue = (*RedisQueue)(nil)

type RedisQueue struct {
	client *redis.Client
}

func NewRedisQueue(client *redis.Client) *RedisQueue {
	return &RedisQueue{client: client}
}

// Push adds a job ID to the left of the Redis list.
func (q *RedisQueue) Push(ctx context.Context, jobID uuid.UUID) error {
	err := q.client.LPush(ctx, jobQueueKey, jobID.String()).Err()
	if err != nil {
		return fmt.Errorf("failed to push job to queue: %w", err)
	}
	return nil
}

// Pop blocks until a job ID is available, then removes and returns it.
// timeout controls how long to wait — 0 means block forever.
func (q *RedisQueue) Pop(ctx context.Context, timeout time.Duration) (uuid.UUID, error) {
	result, err := q.client.BRPop(ctx, timeout, jobQueueKey).Result()
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to pop job from queue: %w", err)
	}

	// BRPop returns [key, value]
	jobID, err := uuid.Parse(result[1])
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to parse job id: %w", err)
	}

	return jobID, nil
}

// Len returns the current number of jobs in the queue.
func (q *RedisQueue) Len(ctx context.Context) (int64, error) {
	length, err := q.client.LLen(ctx, jobQueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue length: %w", err)
	}
	return length, nil
}
