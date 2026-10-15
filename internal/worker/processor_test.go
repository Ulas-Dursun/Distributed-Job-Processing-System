package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/ulasdursun/queueforge/internal/domain"
	"github.com/ulasdursun/queueforge/internal/worker"
)

func TestValidateTargetURL(t *testing.T) {
	// Ensure default SSRF enforcement is active during this test
	os.Unsetenv("ALLOW_LOCAL_FETCH")

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid public https URL",
			url:     "https://example.com",
			wantErr: false,
		},
		{
			name:    "unsupported scheme ftp",
			url:     "ftp://example.com/file",
			wantErr: true,
		},
		{
			name:    "unsupported scheme file",
			url:     "file:///etc/passwd",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "SSRF blocked: loopback IPv4",
			url:     "http://127.0.0.1:8080/metrics",
			wantErr: true,
		},
		{
			name:    "SSRF blocked: private RFC1918 10.x.x.x",
			url:     "http://10.0.0.1/admin",
			wantErr: true,
		},
		{
			name:    "SSRF blocked: private RFC1918 192.168.x.x",
			url:     "http://192.168.1.1/router",
			wantErr: true,
		},
		{
			name:    "SSRF blocked: AWS instance metadata",
			url:     "http://169.254.169.254/latest/meta-data/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := worker.ValidateTargetURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTargetURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestProcess_URLFetchSuccess(t *testing.T) {
	os.Setenv("ALLOW_LOCAL_FETCH", "true")
	defer os.Unsetenv("ALLOW_LOCAL_FETCH")

	expectedText := "Go is an open source programming language supported by Google"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, expectedText)
	}))
	defer server.Close()

	payloadJSON, _ := json.Marshal(worker.URLFetchPayload{URL: server.URL})

	job := &domain.Job{
		ID:      uuid.New(),
		Type:    "URL_FETCH",
		Payload: string(payloadJSON),
	}

	resultStr, err := worker.Process(context.Background(), job)
	if err != nil {
		t.Fatalf("Process returned unexpected error: %v", err)
	}

	var result worker.URLFetchResult
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		t.Fatalf("Failed to unmarshal result JSON: %v", err)
	}

	if result.WordCount != 10 {
		t.Errorf("WordCount = %d, want 10", result.WordCount)
	}

	if result.URL != server.URL {
		t.Errorf("Result URL = %q, want %q", result.URL, server.URL)
	}
}

func TestProcess_UpstreamServerError(t *testing.T) {
	os.Setenv("ALLOW_LOCAL_FETCH", "true")
	defer os.Unsetenv("ALLOW_LOCAL_FETCH")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	payloadJSON, _ := json.Marshal(worker.URLFetchPayload{URL: server.URL})

	job := &domain.Job{
		ID:      uuid.New(),
		Type:    "URL_FETCH",
		Payload: string(payloadJSON),
	}

	_, err := worker.Process(context.Background(), job)
	if err == nil {
		t.Fatal("Expected error for 500 status code, got nil")
	}
}

func TestProcess_InvalidJobType(t *testing.T) {
	job := &domain.Job{
		ID:   uuid.New(),
		Type: "UNSUPPORTED_TYPE",
	}

	_, err := worker.Process(context.Background(), job)
	if err != domain.ErrInvalidJobType {
		t.Errorf("Expected ErrInvalidJobType, got %v", err)
	}
}
