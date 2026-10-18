package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ulasdursun/queueforge/internal/domain"
	"github.com/ulasdursun/queueforge/internal/metrics"
	"github.com/ulasdursun/queueforge/internal/queue"
	"github.com/ulasdursun/queueforge/internal/repository"
	"github.com/ulasdursun/queueforge/internal/worker"
)

type JobHandler struct {
	jobRepo    *repository.JobRepository
	queue      *queue.RedisQueue
	maxRetries int
}

func NewJobHandler(
	jobRepo *repository.JobRepository,
	q *queue.RedisQueue,
	maxRetries int,
) *JobHandler {
	return &JobHandler{
		jobRepo:    jobRepo,
		queue:      q,
		maxRetries: maxRetries,
	}
}

// CreateJobRequest is the expected request body for job creation.
type CreateJobRequest struct {
	Type    string `json:"type"    binding:"required"`
	Payload string `json:"payload" binding:"required"`
}

// CreateJob handles incoming job creation requests.
func (h *JobHandler) CreateJob(c *gin.Context) {
	var req CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Validation failed",
			"message": err.Error(),
		})
		return
	}

	if req.Type != "URL_FETCH" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid job type",
			"message": "supported types: URL_FETCH",
		})
		return
	}

	// Early fail-fast validation for payload
	var p worker.URLFetchPayload
	if err := json.Unmarshal([]byte(req.Payload), &p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid payload format",
			"message": "payload must be valid JSON: {\"url\": \"https://...\"}",
		})
		return
	}

	if err := worker.ValidateTargetURL(p.URL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid URL or security restriction",
			"message": err.Error(),
		})
		return
	}

	now := time.Now()
	job := &domain.Job{
		ID:         uuid.New(),
		Type:       req.Type,
		Payload:    req.Payload,
		Status:     domain.StatusQueued,
		Attempts:   0,
		MaxRetries: h.maxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := h.jobRepo.Create(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to create job",
			"message": err.Error(),
		})
		return
	}

	if err := h.queue.Push(c.Request.Context(), job.ID); err != nil {
		// Prevent zombie QUEUED jobs: mark FAILED in DB if Redis push fails
		job.Status = domain.StatusFailed
		errMsg := fmt.Sprintf("Failed to push to queue: %v", err)
		job.ErrorMsg = &errMsg
		_ = h.jobRepo.UpdateAfterProcessing(job)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to queue job",
			"message": err.Error(),
		})
		return
	}

	metrics.JobsTotal.WithLabelValues("queued").Inc()
	c.JSON(http.StatusCreated, job)
}

// GetJob fetches a job by UUID.
func (h *JobHandler) GetJob(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid job ID",
			"message": "must be a valid UUID",
		})
		return
	}

	job, err := h.jobRepo.GetByID(id)
	if err != nil {
		if err == domain.ErrJobNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Not found",
				"message": "job not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get job",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, job)
}

// ListJobs returns jobs filtered by optional status with pagination.
func (h *JobHandler) ListJobs(c *gin.Context) {
	status := strings.ToUpper(strings.TrimSpace(c.Query("status")))
	if status != "" {
		switch domain.JobStatus(status) {
		case domain.StatusQueued, domain.StatusProcessing, domain.StatusCompleted, domain.StatusFailed:
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid status filter",
				"message": "allowed values: QUEUED, PROCESSING, COMPLETED, FAILED",
			})
			return
		}
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	jobs, err := h.jobRepo.List(status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to list jobs",
			"message": err.Error(),
		})
		return
	}

	if jobs == nil {
		jobs = []*domain.Job{}
	}

	c.JSON(http.StatusOK, gin.H{
		"count":  len(jobs),
		"limit":  limit,
		"offset": offset,
		"jobs":   jobs,
	})
}
