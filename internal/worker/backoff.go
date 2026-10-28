package worker

import (
	"math"
	"time"
)

// CalculateBackoff computes exponential backoff duration based on the attempt count.
// Formula: 2^attempt seconds.
func CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 1 * time.Second
	}
	return time.Duration(math.Pow(2, float64(attempt))) * time.Second
}
