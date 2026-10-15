package worker

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/ulasdursun/queueforge/internal/domain"
	"github.com/ulasdursun/queueforge/internal/metrics"
	"github.com/ulasdursun/queueforge/internal/queue"
	"github.com/ulasdursun/queueforge/internal/repository"
)

// Pool manages a fixed number of worker goroutines.
type Pool struct {
	workerCount int
	queue       *queue.RedisQueue
	jobRepo     *repository.JobRepository
	wg          sync.WaitGroup
}

func NewPool(
	workerCount int,
	q *queue.RedisQueue,
	jobRepo *repository.JobRepository,
) *Pool {
	return &Pool{
		workerCount: workerCount,
		queue:       q,
		jobRepo:     jobRepo,
	}
}

// Start launches N worker goroutines and blocks until ctx is cancelled and workers exit.
func (p *Pool) Start(ctx context.Context) {
	log.Info().Int("workers", p.workerCount).Msg("Starting worker pool")

	// Periodically update queue size metric
	go p.monitorQueue(ctx)

	p.wg.Add(p.workerCount)
	for i := range p.workerCount {
		go p.runWorker(ctx, i+1)
	}

	// Block until context is cancelled (SIGTERM/SIGINT received)
	<-ctx.Done()
	log.Info().Msg("Worker pool shutting down, waiting for in-flight jobs to complete...")
	p.wg.Wait()
	log.Info().Msg("All workers stopped cleanly")
}

// monitorQueue periodically updates the QueueSize Prometheus metric.
func (p *Pool) monitorQueue(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if length, err := p.queue.Len(ctx); err == nil {
				metrics.QueueSize.Set(float64(length))
			}
		}
	}
}

// runWorker is the main loop for a single worker goroutine with panic recovery.
func (p *Pool) runWorker(ctx context.Context, id int) {
	defer p.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Error().Int("worker_id", id).Interface("recover", r).Msg("Worker recovered from panic")
		}
	}()

	log.Info().Int("worker_id", id).Msg("Worker started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Int("worker_id", id).Msg("Worker stopped")
			return
		default:
			p.processNext(ctx, id)
		}
	}
}

// processNext pops one job from the queue and processes it.
func (p *Pool) processNext(ctx context.Context, workerID int) {
	// BRPOP with 1s timeout so ctx.Done() is checked regularly
	jobID, err := p.queue.Pop(ctx, 1*time.Second)
	if err != nil {
		// Context cancelled or timeout — not an error
		return
	}

	metrics.ActiveWorkers.Inc()
	defer metrics.ActiveWorkers.Dec()

	if err := p.handle(ctx, jobID, workerID); err != nil {
		log.Error().
			Int("worker_id", workerID).
			Str("job_id", jobID.String()).
			Err(err).
			Msg("Failed to handle job")
	}
}

// handle fetches the job, processes it, applies retry logic, and persists the result.
func (p *Pool) handle(ctx context.Context, jobID uuid.UUID, workerID int) error {
	job, err := p.jobRepo.GetByID(jobID)
	if err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	log.Info().
		Int("worker_id", workerID).
		Str("job_id", job.ID.String()).
		Str("type", job.Type).
		Msg("Processing job")

	// Mark as PROCESSING
	if err := p.jobRepo.UpdateStatus(job.ID, domain.StatusProcessing); err != nil {
		return fmt.Errorf("failed to mark processing: %w", err)
	}

	start := time.Now()
	job.Attempts++

	result, err := Process(ctx, job)

	duration := time.Since(start).Seconds()
	metrics.JobDuration.Observe(duration)

	if err != nil {
		errMsg := err.Error()
		job.ErrorMsg = &errMsg

		if job.CanRetry() {
			// Exponential backoff before re-queuing
			backoff := time.Duration(math.Pow(2, float64(job.Attempts))) * time.Second
			log.Warn().
				Int("worker_id", workerID).
				Str("job_id", job.ID.String()).
				Int("attempt", job.Attempts).
				Dur("backoff", backoff).
				Err(err).
				Msg("Job failed, scheduled for retry")

			job.Status = domain.StatusQueued
			if err := p.jobRepo.UpdateAfterProcessing(job); err != nil {
				return err
			}

			// Non-blocking backoff: re-queue after backoff in a separate goroutine
			// so that this worker goroutine is freed immediately for other jobs.
			go func(retryID uuid.UUID, delay time.Duration) {
				time.Sleep(delay)
				if err := p.queue.Push(context.Background(), retryID); err != nil {
					log.Error().
						Str("job_id", retryID.String()).
						Err(err).
						Msg("Failed to re-queue job after backoff")
				}
			}(job.ID, backoff)

			metrics.JobsTotal.WithLabelValues("retried").Inc()
			return nil
		}

		// No retries left — mark as FAILED
		job.Status = domain.StatusFailed
		metrics.JobsTotal.WithLabelValues("failed").Inc()

		log.Error().
			Int("worker_id", workerID).
			Str("job_id", job.ID.String()).
			Int("attempts", job.Attempts).
			Msg("Job permanently failed")
	} else {
		job.Status = domain.StatusCompleted
		job.Result = &result
		metrics.JobsTotal.WithLabelValues("completed").Inc()

		log.Info().
			Int("worker_id", workerID).
			Str("job_id", job.ID.String()).
			Float64("duration_seconds", duration).
			Msg("Job completed")
	}

	return p.jobRepo.UpdateAfterProcessing(job)
}
