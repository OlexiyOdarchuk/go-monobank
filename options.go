package monobank

import (
	"net/http"
	"time"
)

// Option configures a [Client]. Pass options to [New], [NewPersonal] or
// [NewCorporate]. Designed to be additive — future options slot in without
// breaking existing callers.
type Option func(*Client)

// WithHTTPClient sets a custom *http.Client. If nil or not provided,
// http.DefaultClient is used.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient == nil {
			return
		}
		c.http = httpClient
	}
}

// WithHTTPDoer accepts any [HTTPDoer]. Useful for plugging in middlewares
// (circuit breakers, custom transports, test fakes).
func WithHTTPDoer(d HTTPDoer) Option {
	return func(c *Client) {
		if d == nil {
			return
		}
		c.http = d
	}
}

// WithBaseURL overrides the default https://api.monobank.ua base URL.
// Useful for testing against a recorded server or monobank's sandbox.
func WithBaseURL(uri string) Option {
	return func(c *Client) {
		c.WithBaseURL(uri)
	}
}

// WithRetry enables automatic retry for transient HTTP failures (5xx and
// 429). Backoff is exponential with full jitter; the Retry-After response
// header is honoured when present.
//
// Pass attempts == 0 to inherit defaults (4 attempts, 500ms base, 30s
// ceiling). Pass attempts <= 1 to disable retry explicitly. Non-positive
// baseDelay / maxDelay inherit defaults.
func WithRetry(attempts int, baseDelay, maxDelay time.Duration) Option {
	return func(c *Client) {
		if attempts == 0 {
			c.retry = defaultRetry
			return
		}
		c.retry.maxAttempts = attempts
		if baseDelay > 0 {
			c.retry.baseDelay = baseDelay
		} else if c.retry.baseDelay == 0 {
			c.retry.baseDelay = defaultRetry.baseDelay
		}
		if maxDelay > 0 {
			c.retry.maxDelay = maxDelay
		} else if c.retry.maxDelay == 0 {
			c.retry.maxDelay = defaultRetry.maxDelay
		}
	}
}

// New returns a public [Client] built from the supplied options. Extensible
// counterpart to [NewClient].
//
//	c := monobank.New(
//	    monobank.WithHTTPClient(myHTTP),
//	    monobank.WithRetry(5, 0, 0),
//	)
func New(opts ...Option) Client {
	c := NewClient(nil)
	for _, opt := range opts {
		opt(&c)
	}
	return c
}
