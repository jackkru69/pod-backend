package load_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Load test for game list query (T134)
// Verify SC-005: Game list query <500ms for 1000 games

const (
	baseURL          = "http://localhost:8090"
	gameListEndpoint = "/api/v1/games"
	targetLatency    = 500 * time.Millisecond
	concurrentUsers  = 50
	requestsPerUser  = 20
)

type GameListResponse struct {
	Games []struct {
		GameID    int64 `json:"game_id"`
		Status    int   `json:"status"`
		BetAmount int64 `json:"bet_amount"`
	} `json:"games"`
}

// TestGameListLoad_Latency tests game list query latency under load
func TestGameListLoad_Latency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	t.Logf("Starting game list load test: %d users x %d requests", concurrentUsers, requestsPerUser)

	var (
		totalRequests      int64
		successfulRequests int64
		failedRequests     int64
	)

	latencies := make([]time.Duration, 0, concurrentUsers*requestsPerUser)
	var latencyMu sync.Mutex

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	var wg sync.WaitGroup

	// Simulate concurrent users
	for userID := 0; userID < concurrentUsers; userID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for req := 0; req < requestsPerUser; req++ {
				atomic.AddInt64(&totalRequests, 1)

				startTime := time.Now()

				// Make request to game list endpoint
				url := fmt.Sprintf("%s%s?status=1&limit=100", baseURL, gameListEndpoint)
				resp, err := client.Get(url)

				latency := time.Since(startTime)

				if err != nil {
					atomic.AddInt64(&failedRequests, 1)
					t.Logf("User %d request %d failed: %v", id, req, err)
					continue
				}

				// Read and close response body
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					atomic.AddInt64(&failedRequests, 1)
					t.Logf("User %d request %d failed with status %d", id, req, resp.StatusCode)
					continue
				}

				// Parse response to ensure it's valid
				var gameResp GameListResponse
				if err := json.Unmarshal(body, &gameResp); err != nil {
					atomic.AddInt64(&failedRequests, 1)
					continue
				}

				atomic.AddInt64(&successfulRequests, 1)

				// Store latency
				latencyMu.Lock()
				latencies = append(latencies, latency)
				latencyMu.Unlock()

				// Small delay between requests from same user
				time.Sleep(100 * time.Millisecond)
			}
		}(userID)

		// Stagger user start times
		time.Sleep(50 * time.Millisecond)
	}

	wg.Wait()

	// Calculate statistics
	var (
		minLatency   = time.Hour
		maxLatency   = time.Duration(0)
		totalLatency = time.Duration(0)
		under500ms   int64
	)

	for _, lat := range latencies {
		totalLatency += lat
		if lat < minLatency {
			minLatency = lat
		}
		if lat > maxLatency {
			maxLatency = lat
		}
		if lat < targetLatency {
			under500ms++
		}
	}

	avgLatency := time.Duration(0)
	if len(latencies) > 0 {
		avgLatency = totalLatency / time.Duration(len(latencies))
	}

	// Calculate percentiles
	p50, p95, p99 := calculatePercentiles(latencies)

	// Report results
	t.Logf("=== Game List Load Test Results ===")
	t.Logf("Total requests: %d", totalRequests)
	t.Logf("Successful requests: %d", successfulRequests)
	t.Logf("Failed requests: %d", failedRequests)
	t.Logf("Min latency: %v", minLatency)
	t.Logf("Max latency: %v", maxLatency)
	t.Logf("Avg latency: %v", avgLatency)
	t.Logf("P50 latency: %v", p50)
	t.Logf("P95 latency: %v", p95)
	t.Logf("P99 latency: %v", p99)
	t.Logf("Requests under 500ms: %d (%.2f%%)", under500ms,
		float64(under500ms)/float64(len(latencies))*100)

	// Assertions for SC-005
	successRate := float64(successfulRequests) / float64(totalRequests) * 100
	assert.GreaterOrEqual(t, successRate, 95.0, "Success rate should be at least 95%")

	assert.Less(t, p95, targetLatency,
		"SC-005: P95 latency should be under 500ms")

	percentUnder500ms := float64(under500ms) / float64(len(latencies)) * 100
	assert.GreaterOrEqual(t, percentUnder500ms, 90.0,
		"SC-005: At least 90% of requests should complete under 500ms")
}

// TestGameListLoad_HighConcurrency tests with many concurrent users
func TestGameListLoad_HighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const users = 100
	const duration = 30 * time.Second

	t.Logf("Testing high concurrency: %d users for %v", users, duration)

	var (
		totalRequests int64
		successCount  int64
		errorCount    int64
	)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	client := &http.Client{Timeout: 5 * time.Second}

	var wg sync.WaitGroup

	for i := 0; i < users; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					atomic.AddInt64(&totalRequests, 1)

					url := fmt.Sprintf("%s%s?status=1&limit=50", baseURL, gameListEndpoint)
					resp, err := client.Get(url)

					if err != nil {
						atomic.AddInt64(&errorCount, 1)
						continue
					}

					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()

					if resp.StatusCode == http.StatusOK {
						atomic.AddInt64(&successCount, 1)
					} else {
						atomic.AddInt64(&errorCount, 1)
					}

					time.Sleep(200 * time.Millisecond)
				}
			}
		}()
	}

	<-ctx.Done()
	time.Sleep(1 * time.Second)
	wg.Wait()

	requestsPerSecond := float64(totalRequests) / duration.Seconds()
	successRate := float64(successCount) / float64(totalRequests) * 100

	t.Logf("=== High Concurrency Test Results ===")
	t.Logf("Duration: %v", duration)
	t.Logf("Total requests: %d", totalRequests)
	t.Logf("Successful: %d", successCount)
	t.Logf("Errors: %d", errorCount)
	t.Logf("Requests/second: %.2f", requestsPerSecond)
	t.Logf("Success rate: %.2f%%", successRate)

	assert.GreaterOrEqual(t, successRate, 95.0,
		"Success rate should be at least 95% under high concurrency")

	assert.GreaterOrEqual(t, requestsPerSecond, 50.0,
		"Should handle at least 50 requests per second")
}

// TestGameListLoad_Pagination tests pagination performance
func TestGameListLoad_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	client := &http.Client{Timeout: 5 * time.Second}

	const pages = 10
	const pageSize = 100

	t.Logf("Testing pagination: %d pages of %d items", pages, pageSize)

	latencies := make([]time.Duration, 0, pages)

	for page := 0; page < pages; page++ {
		offset := page * pageSize

		startTime := time.Now()

		url := fmt.Sprintf("%s%s?status=1&limit=%d&offset=%d",
			baseURL, gameListEndpoint, pageSize, offset)

		resp, err := client.Get(url)
		latency := time.Since(startTime)

		if err != nil {
			t.Logf("Page %d failed: %v", page, err)
			continue
		}

		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			latencies = append(latencies, latency)
			t.Logf("Page %d (offset %d): %v", page, offset, latency)
		}
	}

	// Calculate average latency for pagination
	var totalLatency time.Duration
	for _, lat := range latencies {
		totalLatency += lat
	}

	avgLatency := time.Duration(0)
	if len(latencies) > 0 {
		avgLatency = totalLatency / time.Duration(len(latencies))
	}

	t.Logf("=== Pagination Test Results ===")
	t.Logf("Pages tested: %d", len(latencies))
	t.Logf("Average latency: %v", avgLatency)

	assert.Less(t, avgLatency, targetLatency,
		"Average pagination latency should be under 500ms")
}

// Helper function to calculate percentiles
func calculatePercentiles(latencies []time.Duration) (p50, p95, p99 time.Duration) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}

	// Sort latencies
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)

	// Simple bubble sort (good enough for tests)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p50 = sorted[len(sorted)*50/100]
	p95 = sorted[len(sorted)*95/100]
	p99 = sorted[len(sorted)*99/100]

	return p50, p95, p99
}

// BenchmarkGameList benchmarks the game list endpoint
func BenchmarkGameList(b *testing.B) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("%s%s?status=1&limit=100", baseURL, gameListEndpoint)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, err := client.Get(url)
		if err != nil {
			b.Errorf("Request failed: %v", err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}
