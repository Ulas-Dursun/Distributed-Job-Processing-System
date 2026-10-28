package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ulasdursun/queueforge/internal/api/handler"
	"github.com/ulasdursun/queueforge/internal/domain"
)

// MockJobRepository implements domain.JobRepository for testing.
type MockJobRepository struct {
	CreateFn                func(ctx context.Context, job *domain.Job) error
	GetByIDFn               func(ctx context.Context, id uuid.UUID) (*domain.Job, error)
	ListFn                  func(ctx context.Context, status string, limit int, offset int) ([]*domain.Job, error)
	UpdateStatusFn          func(ctx context.Context, id uuid.UUID, status domain.JobStatus) error
	UpdateAfterProcessingFn func(ctx context.Context, job *domain.Job) error
}

func (m *MockJobRepository) Create(ctx context.Context, job *domain.Job) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, job)
	}
	return nil
}

func (m *MockJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, id)
	}
	return nil, domain.ErrJobNotFound
}

func (m *MockJobRepository) List(ctx context.Context, status string, limit int, offset int) ([]*domain.Job, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, status, limit, offset)
	}
	return []*domain.Job{}, nil
}

func (m *MockJobRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.JobStatus) error {
	if m.UpdateStatusFn != nil {
		return m.UpdateStatusFn(ctx, id, status)
	}
	return nil
}

func (m *MockJobRepository) UpdateAfterProcessing(ctx context.Context, job *domain.Job) error {
	if m.UpdateAfterProcessingFn != nil {
		return m.UpdateAfterProcessingFn(ctx, job)
	}
	return nil
}

// MockJobQueue implements domain.JobQueue for testing.
type MockJobQueue struct {
	PushFn func(ctx context.Context, jobID uuid.UUID) error
	PopFn  func(ctx context.Context, timeout time.Duration) (uuid.UUID, error)
	LenFn  func(ctx context.Context) (int64, error)
}

func (m *MockJobQueue) Push(ctx context.Context, jobID uuid.UUID) error {
	if m.PushFn != nil {
		return m.PushFn(ctx, jobID)
	}
	return nil
}

func (m *MockJobQueue) Pop(ctx context.Context, timeout time.Duration) (uuid.UUID, error) {
	if m.PopFn != nil {
		return m.PopFn(ctx, timeout)
	}
	return uuid.Nil, nil
}

func (m *MockJobQueue) Len(ctx context.Context) (int64, error) {
	if m.LenFn != nil {
		return m.LenFn(ctx)
	}
	return 0, nil
}

func setupTestRouter(h *handler.JobHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	jobs := r.Group("/api/jobs")
	{
		jobs.POST("", h.CreateJob)
		jobs.GET("", h.ListJobs)
		jobs.GET("/:id", h.GetJob)
	}
	return r
}

func TestCreateJob_Success(t *testing.T) {
	os.Setenv("ALLOW_LOCAL_FETCH", "true")
	defer os.Unsetenv("ALLOW_LOCAL_FETCH")

	repo := &MockJobRepository{}
	q := &MockJobQueue{}
	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	body := `{"type":"URL_FETCH","payload":"{\"url\":\"https://example.com\"}"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var res domain.Job
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if res.Type != domain.JobTypeURLFetch {
		t.Errorf("expected type URL_FETCH, got %s", res.Type)
	}
	if res.Status != domain.StatusQueued {
		t.Errorf("expected status QUEUED, got %s", res.Status)
	}
}

func TestCreateJob_InvalidJobType(t *testing.T) {
	repo := &MockJobRepository{}
	q := &MockJobQueue{}
	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	body := `{"type":"INVALID_TYPE","payload":"{\"url\":\"https://example.com\"}"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestCreateJob_SSRFBlocked(t *testing.T) {
	os.Unsetenv("ALLOW_LOCAL_FETCH")

	repo := &MockJobRepository{}
	q := &MockJobQueue{}
	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	body := `{"type":"URL_FETCH","payload":"{\"url\":\"http://127.0.0.1:8080/metrics\"}"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for SSRF target, got %d", w.Code)
	}
}

func TestCreateJob_QueuePushFailure_Rollback(t *testing.T) {
	os.Setenv("ALLOW_LOCAL_FETCH", "true")
	defer os.Unsetenv("ALLOW_LOCAL_FETCH")

	var rollbackStatus domain.JobStatus
	repo := &MockJobRepository{
		UpdateAfterProcessingFn: func(ctx context.Context, job *domain.Job) error {
			rollbackStatus = job.Status
			return nil
		},
	}
	q := &MockJobQueue{
		PushFn: func(ctx context.Context, jobID uuid.UUID) error {
			return errors.New("redis connection refused")
		},
	}

	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	body := `{"type":"URL_FETCH","payload":"{\"url\":\"https://example.com\"}"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
	}

	if rollbackStatus != domain.StatusFailed {
		t.Errorf("expected job to be marked FAILED on queue error, got %s", rollbackStatus)
	}
}

func TestGetJob_Found(t *testing.T) {
	jobID := uuid.New()
	repo := &MockJobRepository{
		GetByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
			if id == jobID {
				return &domain.Job{
					ID:     jobID,
					Type:   domain.JobTypeURLFetch,
					Status: domain.StatusCompleted,
				}, nil
			}
			return nil, domain.ErrJobNotFound
		},
	}
	q := &MockJobQueue{}
	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	req, _ := http.NewRequest(http.MethodGet, "/api/jobs/"+jobID.String(), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	repo := &MockJobRepository{
		GetByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
			return nil, domain.ErrJobNotFound
		},
	}
	q := &MockJobQueue{}
	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	req, _ := http.NewRequest(http.MethodGet, "/api/jobs/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestGetJob_InvalidUUID(t *testing.T) {
	repo := &MockJobRepository{}
	q := &MockJobQueue{}
	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	req, _ := http.NewRequest(http.MethodGet, "/api/jobs/not-a-uuid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestListJobs_InvalidStatusFilter(t *testing.T) {
	repo := &MockJobRepository{}
	q := &MockJobQueue{}
	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	req, _ := http.NewRequest(http.MethodGet, "/api/jobs?status=INVALID_STATUS", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestListJobs_Success(t *testing.T) {
	repo := &MockJobRepository{
		ListFn: func(ctx context.Context, status string, limit int, offset int) ([]*domain.Job, error) {
			return []*domain.Job{
				{ID: uuid.New(), Type: domain.JobTypeURLFetch, Status: domain.StatusCompleted},
			}, nil
		},
	}
	q := &MockJobQueue{}
	h := handler.NewJobHandler(repo, q, 3)
	router := setupTestRouter(h)

	req, _ := http.NewRequest(http.MethodGet, "/api/jobs?status=COMPLETED&limit=10&offset=0", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}
