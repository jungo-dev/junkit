package httpclient

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// retryTransport wraps an http.RoundTripper to automatically retry failed requests
// (network errors, HTTP 429, and 5xx responses) with exponential backoff and jitter.
type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
	waitMin    time.Duration
	waitMax    time.Duration
}

// RoundTrip executes an HTTP request, retrying transient failures up to maxRetries times.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	bodyBytes, err := drainBody(req)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			resetBody(req, bodyBytes)
		}

		resp, err = t.base.RoundTrip(req)
		if !t.shouldRetry(resp, err) || attempt >= t.maxRetries {
			return resp, err
		}

		if resp != nil {
			_ = resp.Body.Close()
		}

		if waitErr := sleepWithContext(req.Context(), t.backoff(attempt)); waitErr != nil {
			return nil, waitErr
		}
	}
}

// shouldRetry checks if a request should be retried (network error, 429, or 5xx status).
func (t *retryTransport) shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
}

// backoff calculates the delay before the next retry using exponential backoff with jitter.
func (t *retryTransport) backoff(attempt int) time.Duration {
	wait := t.waitMin << attempt // exponential: waitMin, 2*waitMin, 4*waitMin, ...
	if wait <= 0 || wait > t.waitMax {
		wait = t.waitMax
	}
	jitter := time.Duration(rand.Int63n(int64(wait)/5 + 1))
	return wait + jitter
}

// drainBody reads and buffers req.Body into memory so it can be reused across retries.
func drainBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	b, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	resetBody(req, b)
	return b, nil
}

// resetBody resets req.Body with the buffered bytes for the next retry attempt.
func resetBody(req *http.Request, body []byte) {
	if body == nil {
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}

// sleepWithContext pauses execution for duration d unless context is canceled or times out.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
