package session

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// apiKeyConfig is a minimal SessionConfig used to drive the API-key branch of
// Authenticate. It is deliberately separate from authentication_test.go's
// config, which implements a different interface.
type apiKeyConfig struct {
	apiKey   string
	username string
}

func (c *apiKeyConfig) GetUsername() string                  { return c.username }
func (c *apiKeyConfig) GetAPIKey() string                    { return c.apiKey }
func (c *apiKeyConfig) GetSessionStoreKey() []byte           { return []byte("0123456789abcdef0123456789abcdef") }
func (c *apiKeyConfig) GetMaxSessionAge() int                { return 0 }
func (c *apiKeyConfig) ValidateCredentials(_, _ string) bool { return false }

// TestAuthenticateAPIKey covers the constant-time API-key comparison in
// Authenticate: the correct key (via header or query param) authenticates as
// the configured user, and any mismatch — including an empty configured key —
// is rejected with ErrUnauthorized.
func TestAuthenticateAPIKey(t *testing.T) {
	const key = "s3cr3t-api-key-value"

	newReq := func(headerKey, queryKey string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/graphql", nil)
		if headerKey != "" {
			r.Header.Set(ApiKeyHeader, headerKey)
		}
		if queryKey != "" {
			q := r.URL.Query()
			q.Set(ApiKeyParameter, queryKey)
			r.URL.RawQuery = q.Encode()
		}
		return r
	}

	cases := []struct {
		name          string
		configuredKey string
		header        string
		query         string
		wantUser      string
		wantErr       bool
	}{
		{"correct key via header", key, key, "", "admin", false},
		{"correct key via query param", key, "", key, "admin", false},
		{"wrong key via header", key, "wrong-key", "", "", true},
		{"wrong-length key via header", key, "x", "", "", true},
		{"empty configured key rejects presented key", "", key, "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore(&apiKeyConfig{apiKey: tc.configuredKey, username: "admin"})
			userID, err := store.Authenticate(httptest.NewRecorder(), newReq(tc.header, tc.query))

			if tc.wantErr {
				if !errors.Is(err, ErrUnauthorized) {
					t.Fatalf("expected ErrUnauthorized, got user=%q err=%v", userID, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if userID != tc.wantUser {
				t.Fatalf("expected user %q, got %q", tc.wantUser, userID)
			}
		})
	}
}
