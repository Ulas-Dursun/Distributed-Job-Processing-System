package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Result struct {
	Duration time.Duration
	Err      error
	Status   int
}

func main() {
	targetBase := flag.String("url", "http://localhost:8080", "QueueForge base URL")
	concurrency := flag.Int("c", 50, "Concurrent workers")
	totalRequests := flag.Int("n", 500, "Total number of job submissions")
	flag.Parse()

	endpoint := fmt.Sprintf("%s/api/jobs", *targetBase)

	fmt.Printf("\n🚀 Starting QueueForge Load Test\n")
	fmt.Printf("   Target Endpoint: %s\n", endpoint)
	fmt.Printf("   Total Submissions: %d\n", *totalRequests)
	fmt.Printf("   Concurrency Level: %d workers\n\n", *concurrency)

	payloadBytes, _ := json.Marshal(map[string]string{
		"type":    "URL_FETCH",
		"payload": `{"url":"https://example.com"}`,
	})

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		},
	}

	jobsChan := make(chan int, *totalRequests)
	for i := 1; i <= *totalRequests; i++ {
		jobsChan <- i
	}
	close(jobsChan)

	results := make([]Result, *totalRequests)
	var resultIdx int64 = 0

	var successCount int64 = 0
	var failCount int64 = 0

	startTime := time.Now()
	var wg sync.WaitGroup

	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobsChan {
				t0 := time.Now()
				resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payloadBytes))
				dur := time.Since(t0)

				statusCode := 0
				if err == nil {
					statusCode = resp.StatusCode
					resp.Body.Close()
					if statusCode == http.StatusCreated {
						atomic.AddInt64(&successCount, 1)
					} else {
						atomic.AddInt64(&failCount, 1)
					}
				} else {
					atomic.AddInt64(&failCount, 1)
				}

				idx := atomic.AddInt64(&resultIdx, 1) - 1
				results[idx] = Result{
					Duration: dur,
					Err:      err,
					Status:   statusCode,
				}
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Collect and sort durations for percentile calculations
	durations := make([]time.Duration, 0, *totalRequests)
	for _, r := range results {
		durations = append(durations, r.Duration)
	}
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	var totalLatency time.Duration
	for _, d := range durations {
		totalLatency += d
	}
	avgLatency := totalLatency / time.Duration(len(durations))

	p50 := durations[len(durations)*50/100]
	p90 := durations[len(durations)*90/100]
	p95 := durations[len(durations)*95/100]
	p99 := durations[len(durations)*99/100]
	minLat := durations[0]
	maxLat := durations[len(durations)-1]

	rps := float64(*totalRequests) / totalDuration.Seconds()

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("📊 Load Test Summary Results:\n")
	fmt.Printf("   Total Time Taken:   %v\n", totalDuration.Round(time.Millisecond))
	fmt.Printf("   Throughput:         %.2f req/sec\n", rps)
	fmt.Printf("   Successful (201):   %d (%.1f%%)\n", successCount, float64(successCount)/float64(*totalRequests)*100)
	fmt.Printf("   Failed:             %d\n", failCount)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("⏱  Submission Latency Percentiles:\n")
	fmt.Printf("   Min:                %v\n", minLat.Round(time.Microsecond))
	fmt.Printf("   Average:            %v\n", avgLatency.Round(time.Microsecond))
	fmt.Printf("   p50 (Median):       %v\n", p50.Round(time.Microsecond))
	fmt.Printf("   p90:                %v\n", p90.Round(time.Microsecond))
	fmt.Printf("   p95:                %v\n", p95.Round(time.Microsecond))
	fmt.Printf("   p99:                %v\n", p99.Round(time.Microsecond))
	fmt.Printf("   Max:                %v\n", maxLat.Round(time.Microsecond))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
