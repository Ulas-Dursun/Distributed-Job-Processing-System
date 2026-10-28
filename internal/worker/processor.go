package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ulasdursun/queueforge/internal/domain"
	"github.com/ulasdursun/queueforge/internal/validator"
)

const maxFetchBytes = 2 * 1024 * 1024 // 2MB limit to prevent worker memory exhaustion (OOM)

// URLFetchPayload is an alias to validator.URLFetchPayload for backwards compatibility.
type URLFetchPayload = validator.URLFetchPayload

// URLFetchResult is the output stored after processing.
type URLFetchResult struct {
	URL       string `json:"url"`
	WordCount int    `json:"word_count"`
	FetchedAt string `json:"fetched_at"`
}

// Process executes the job based on its type.
// Returns result JSON string or an error.
func Process(ctx context.Context, job *domain.Job) (string, error) {
	switch job.Type {
	case domain.JobTypeURLFetch:
		return processFetch(ctx, job.Payload)
	default:
		return "", domain.ErrInvalidJobType
	}
}

// ValidateTargetURL delegates to validator.ValidateTargetURL.
func ValidateTargetURL(rawURL string) error {
	return validator.ValidateTargetURL(rawURL)
}

func processFetch(ctx context.Context, payload string) (string, error) {
	var p validator.URLFetchPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return "", fmt.Errorf("invalid payload: %w", err)
	}

	if p.URL == "" {
		return "", domain.ErrInvalidPayload
	}

	if err := validator.ValidateTargetURL(p.URL); err != nil {
		return "", fmt.Errorf("security check failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "QueueForge-Bot/1.0 (+https://github.com/ulasdursun/queueforge)")

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			return validator.ValidateTargetURL(req.URL.String())
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("upstream server returned HTTP %d", resp.StatusCode)
	}

	// Limit reader to 2MB to protect from decompression/infinite stream bombs
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	wordCount := len(strings.Fields(string(body)))

	result := URLFetchResult{
		URL:       p.URL,
		WordCount: wordCount,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
