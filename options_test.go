package monobank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_defaults(t *testing.T) {
	c := New()
	require.NotNil(t, c.baseURL)
	assert.Equal(t, baseURL, c.baseURL.String())
	assert.NotNil(t, c.http)
	assert.NotNil(t, c.auth)
}

func TestNew_withBaseURL(t *testing.T) {
	c := New(WithBaseURL("https://example.test"))
	require.NotNil(t, c.baseURL)
	assert.Equal(t, "https://example.test", c.baseURL.String())
}

func TestNew_withHTTPClient(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	c := New(
		WithHTTPClient(server.Client()),
		WithBaseURL(server.URL),
	)

	_, err := c.Currency(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, hits)
}

func TestNew_nilHTTPClient_isIgnored(t *testing.T) {
	// passing a nil http.Client must not panic and must not wipe the default
	c := New(WithHTTPClient(nil))
	assert.NotNil(t, c.http)
}
