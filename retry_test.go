package monobank

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRetryAfter(t *testing.T) {
	tests := map[string]struct {
		header string
		want   time.Duration
	}{
		"missing":       {"", 0},
		"seconds":       {"5", 5 * time.Second},
		"zero seconds":  {"0", 0},
		"negative":      {"-3", 0},
		"garbage":       {"soon", 0},
		"http-date now": {"Sun, 06 Nov 1994 08:49:37 GMT", 0}, // in the past → 0
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set("Retry-After", tc.header)
			}
			assert.Equal(t, tc.want, parseRetryAfter(h))
		})
	}

	t.Run("http-date future", func(t *testing.T) {
		future := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
		h := http.Header{"Retry-After": []string{future}}
		got := parseRetryAfter(h)
		assert.Greater(t, got, 5*time.Second)
		assert.LessOrEqual(t, got, 10*time.Second)
	})
}

func TestRetryPolicy_disabledByDefault(t *testing.T) {
	var calls atomic.Int32
	err := retryPolicy{}.run(context.Background(), func() error {
		calls.Add(1)
		return errors.New("nope")
	})
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "zero-value policy must not retry")
}

func TestRetryPolicy_retriesTransient(t *testing.T) {
	rp := retryPolicy{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}

	var attempts atomic.Int32
	err := rp.run(context.Background(), func() error {
		n := attempts.Add(1)
		if n < 3 {
			return &transientStatus{code: 503, cause: errors.New("upstream")}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestRetryPolicy_giveUpAfterMaxAttempts(t *testing.T) {
	rp := retryPolicy{maxAttempts: 2, baseDelay: time.Millisecond, maxDelay: 2 * time.Millisecond}

	var attempts atomic.Int32
	err := rp.run(context.Background(), func() error {
		attempts.Add(1)
		return &transientStatus{code: 503, cause: errors.New("still down")}
	})
	require.Error(t, err)
	assert.Equal(t, int32(2), attempts.Load())
	var ts *transientStatus
	assert.ErrorAs(t, err, &ts)
}

func TestRetryPolicy_doesNotRetryPermanent(t *testing.T) {
	rp := retryPolicy{maxAttempts: 5, baseDelay: time.Millisecond, maxDelay: 2 * time.Millisecond}

	var attempts atomic.Int32
	permanent := errors.New("4xx-ish")
	err := rp.run(context.Background(), func() error {
		attempts.Add(1)
		return permanent
	})
	assert.ErrorIs(t, err, permanent)
	assert.Equal(t, int32(1), attempts.Load(), "non-transient errors must not retry")
}

func TestRetryPolicy_respectsContext(t *testing.T) {
	rp := retryPolicy{maxAttempts: 10, baseDelay: 50 * time.Millisecond, maxDelay: time.Second}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := rp.run(ctx, func() error {
		return &transientStatus{code: 503, cause: errors.New("down")}
	})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRetryPolicy_endToEndViaClient_with429AndRetryAfter(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0") // wake up immediately
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	c := New(
		WithHTTPClient(server.Client()),
		WithBaseURL(server.URL),
		WithRetry(3, time.Millisecond, 10*time.Millisecond),
	)
	_, err := c.Currency(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), hits.Load(), "expected one retry after 429")
}

func TestAPIError_carriesStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorDescription":"Missing X-Token"}`))
	}))
	defer server.Close()

	base, _ := url.Parse(server.URL)
	c := Client{baseURL: base, http: server.Client(), auth: NewPublicAuthorizer()}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test", http.NoBody)

	var v any
	err := c.do(req, &v)
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, []int{http.StatusOK}, apiErr.ExpectedStatusCodes)
	assert.Contains(t, string(apiErr.Body), "Missing X-Token")
	assert.Contains(t, apiErr.Error(), "HTTP 400")
}

func TestClient_do_multipleExpectedStatusCodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated) // 201
	}))
	defer server.Close()

	base, _ := url.Parse(server.URL)
	c := Client{baseURL: base, http: server.Client(), auth: NewPublicAuthorizer()}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)

	// Accept 200 or 201:
	err := c.do(req, nil, http.StatusOK, http.StatusCreated)
	require.NoError(t, err)

	// If we only accept 200, the same response fails with APIError:
	req, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	err = c.do(req, nil, http.StatusOK)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusCreated, apiErr.StatusCode)
}

// Make sure transientStatus's Error() string is descriptive.
func TestTransientStatusError(t *testing.T) {
	e := &transientStatus{code: 503, retryAfter: 2 * time.Second, cause: errors.New("upstream")}
	got := e.Error()
	require.Contains(t, got, "503")
	require.Contains(t, got, "2s")

	e2 := &transientStatus{code: 502, cause: fmt.Errorf("bad gateway")}
	require.Contains(t, e2.Error(), "502")
}
