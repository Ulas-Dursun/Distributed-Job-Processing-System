package domain_test

import (
	"testing"

	"github.com/ulasdursun/queueforge/internal/domain"
)

func TestJob_CanRetry(t *testing.T) {
	tests := []struct {
		name       string
		attempts   int
		maxRetries int
		want       bool
	}{
		{
			name:       "new job with 0 attempts can retry",
			attempts:   0,
			maxRetries: 3,
			want:       true,
		},
		{
			name:       "job with 2 attempts can retry when max is 3",
			attempts:   2,
			maxRetries: 3,
			want:       true,
		},
		{
			name:       "job that reached max retries cannot retry",
			attempts:   3,
			maxRetries: 3,
			want:       false,
		},
		{
			name:       "job that exceeded max retries cannot retry",
			attempts:   4,
			maxRetries: 3,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &domain.Job{
				Attempts:   tt.attempts,
				MaxRetries: tt.maxRetries,
			}
			if got := job.CanRetry(); got != tt.want {
				t.Errorf("CanRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}
