package monobank

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
)

const baseURL = "https://api.monobank.ua"

// Errors.
var (
	ErrEmptyRequest = errors.New("empty request")
	ErrInvalidURL   = errors.New("invalid URL")
)

// HTTPDoer is the subset of *http.Client that Client depends on. Any
// transport implementing it (the stdlib client, a custom round-tripped one,
// a test fake) plugs in via [WithHTTPClient].
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// APIError is returned when monobank's HTTP response does not match any of
// the expected status codes.
type APIError struct {
	Method              string
	URL                 string
	StatusCode          int
	ExpectedStatusCodes []int
	Body                []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d (expected %v): %s",
		e.Method, e.URL, e.StatusCode, e.ExpectedStatusCodes, truncate(e.Body, 256))
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// Client is the unauthenticated base client. PersonalClient and
// CorporateClient embed it.
type Client struct {
	http    HTTPDoer
	auth    Authorizer
	baseURL *url.URL
	retry   retryPolicy
}

// NewClient returns a public Client using the given *http.Client (nil → default).
// Prefer [New] for new code.
func NewClient(client *http.Client) Client {
	if client == nil {
		client = &http.Client{}
	}
	base, _ := url.Parse(baseURL)
	return Client{
		http:    client,
		auth:    NewPublicAuthorizer(),
		baseURL: base,
	}
}

// WithBaseURL overrides the base URL. Kept for backwards compatibility;
// new code passes [WithBaseURL] to [New].
func (c *Client) WithBaseURL(uri string) {
	u, err := url.Parse(uri)
	if err != nil || u == nil {
		return
	}
	c.baseURL = u
}

func (c *Client) withAuth(auth Authorizer) {
	c.auth = auth
}

// do executes req against c.baseURL and decodes the JSON response into v.
// Pass any number of expected status codes; defaults to http.StatusOK.
//
// Transient failures (5xx, 429) are retried according to the configured
// retry policy (see [WithRetry]). Context cancellation aborts immediately.
func (c Client) do(req *http.Request, v any, expectedStatusCodes ...int) error {
	if req == nil {
		return ErrEmptyRequest
	}
	if c.baseURL == nil {
		return ErrInvalidURL
	}
	if len(expectedStatusCodes) == 0 {
		expectedStatusCodes = []int{http.StatusOK}
	}

	target, err := url.Parse(req.URL.String())
	if err != nil {
		return fmt.Errorf("parse request URL: %w", err)
	}
	req.URL = c.baseURL.ResolveReference(target)
	if req.Header.Get("Content-Type") == "" && req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.auth != nil {
		if err := c.auth.SetAuth(req); err != nil {
			return fmt.Errorf("SetAuth: %w", err)
		}
	}

	return c.retry.run(req.Context(), func() error {
		return c.attempt(req, v, expectedStatusCodes)
	})
}

func (c Client) attempt(req *http.Request, v any, expectedStatusCodes []int) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !slices.Contains(expectedStatusCodes, resp.StatusCode) {
		body, _ := io.ReadAll(resp.Body)
		apiErr := &APIError{
			Method:              req.Method,
			URL:                 req.URL.String(),
			StatusCode:          resp.StatusCode,
			ExpectedStatusCodes: expectedStatusCodes,
			Body:                body,
		}
		if isTransientStatus(resp.StatusCode) {
			return &transientStatus{
				code:       resp.StatusCode,
				retryAfter: parseRetryAfter(resp.Header),
				cause:      apiErr,
			}
		}
		return apiErr
	}

	if v == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// isTransientStatus reports whether an HTTP status code is worth retrying.
func isTransientStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}
