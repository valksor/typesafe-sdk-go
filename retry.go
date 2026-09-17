package typesafe

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy controls retries after the initial request attempt.
type RetryPolicy struct {
	MaxRetries         int
	BackoffInitial     time.Duration
	BackoffMax         time.Duration
	BackoffJitter      float64
	HTTPStatuses       map[int]struct{}
	RespectRetryAfter  bool
	MaxRetryAfter      time.Duration
	APIConnectionError bool
	APITimeoutError    bool
}

// DefaultRetryPolicy returns an independent copy of the SDK retry defaults.
func DefaultRetryPolicy() RetryPolicy {
	statuses := map[int]struct{}{408: {}, 429: {}}
	for status := 500; status <= 599; status++ {
		statuses[status] = struct{}{}
	}
	return RetryPolicy{
		MaxRetries:         2,
		BackoffInitial:     500 * time.Millisecond,
		BackoffMax:         5 * time.Second,
		BackoffJitter:      0.25,
		HTTPStatuses:       statuses,
		RespectRetryAfter:  true,
		MaxRetryAfter:      time.Minute,
		APIConnectionError: true,
		APITimeoutError:    true,
	}
}

func (p RetryPolicy) validate() error {
	if p.MaxRetries < 0 {
		return fmt.Errorf("retry max retries must be non-negative: %w", ErrInvalidRequest)
	}
	if p.BackoffInitial < 0 || p.BackoffMax < 0 || p.MaxRetryAfter < 0 {
		return fmt.Errorf("retry durations must be non-negative: %w", ErrInvalidRequest)
	}
	if math.IsNaN(p.BackoffJitter) || p.BackoffJitter < 0 || p.BackoffJitter > 1 {
		return fmt.Errorf("retry jitter must be between zero and one: %w", ErrInvalidRequest)
	}
	for status := range p.HTTPStatuses {
		if status < 100 || status > 999 {
			return fmt.Errorf("invalid retry HTTP status %d: %w", status, ErrInvalidRequest)
		}
	}
	return nil
}

func (p RetryPolicy) delay(attempt int, headers http.Header) time.Duration {
	if p.RespectRetryAfter {
		if delay, ok := parseRetryAfter(headers, time.Now()); ok && delay <= p.MaxRetryAfter {
			return delay
		}
	}
	delay := float64(p.BackoffInitial) * math.Pow(2, float64(attempt))
	delay = min(delay, float64(p.BackoffMax))
	delay *= 1 - rand.Float64()*p.BackoffJitter
	return time.Duration(math.Round(delay))
}

func parseRetryAfter(headers http.Header, now time.Time) (time.Duration, bool) {
	if raw := strings.TrimSpace(headers.Get("retry-after-ms")); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
			return time.Duration(value * float64(time.Millisecond)), true
		}
	}
	raw := strings.TrimSpace(headers.Get("retry-after"))
	if raw == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
		if !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds >= 0 {
			return time.Duration(seconds * float64(time.Second)), true
		}
		return 0, false
	}
	date, err := http.ParseTime(raw)
	if err != nil {
		return 0, false
	}
	return max(date.Sub(now), 0), true
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
