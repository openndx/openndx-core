package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// withStubbedBrowser replaces the package-level openBrowser var with fn for
// the duration of the test, restoring the original afterwards.
func withStubbedBrowser(t *testing.T, fn func(rawURL string) error) {
	t.Helper()
	original := openBrowser
	openBrowser = fn
	t.Cleanup(func() { openBrowser = original })
}

// simulateBrowserCallback acts as a stand-in for a human completing the
// login in a real browser: it parses the authorization URL the CLI would
// have opened, and issues the same redirect a real IDP would send back to
// the local callback server.
func simulateBrowserCallback(t *testing.T, authorizeURL string, overrideCode, overrideState *string) error {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	assert.NoError(t, err)
	q := u.Query()

	code := "test-auth-code"
	if overrideCode != nil {
		code = *overrideCode
	}
	state := q.Get("state")
	if overrideState != nil {
		state = *overrideState
	}

	redirectURI := q.Get("redirect_uri")
	cbURL := fmt.Sprintf("%s?code=%s&state=%s", redirectURI, code, state)
	resp, err := http.Get(cbURL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

func newTestTokenServer(t *testing.T, capturedForm chan<- url.Values, response string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		if capturedForm != nil {
			capturedForm <- r.Form
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(response))
	}))
}

func TestLogin_Success(t *testing.T) {
	formCh := make(chan url.Values, 1)
	tokenServer := newTestTokenServer(t, formCh, `{"access_token":"at-123","refresh_token":"rt-456","token_type":"Bearer","expires_in":3600}`, http.StatusOK)
	defer tokenServer.Close()

	withStubbedBrowser(t, func(rawURL string) error {
		return simulateBrowserCallback(t, rawURL, nil, nil)
	})

	token, err := Login(context.Background(), LoginOptions{
		AuthURL:     "https://idp.example.com/oauth2/authorize",
		TokenURL:    tokenServer.URL,
		ClientID:    "cli-client",
		Scopes:      "openid",
		OpenBrowser: true,
		Timeout:     5 * time.Second,
	})

	assert.NoError(t, err)
	if assert.NotNil(t, token) {
		assert.Equal(t, "at-123", token.AccessToken)
		assert.Equal(t, "rt-456", token.RefreshToken)
		assert.Equal(t, "Bearer", token.TokenType)
		assert.Equal(t, tokenServer.URL, token.TokenURL)
		assert.Equal(t, "cli-client", token.ClientID)
		assert.False(t, token.Expired())
	}

	select {
	case form := <-formCh:
		assert.Equal(t, "authorization_code", form.Get("grant_type"))
		assert.Equal(t, "test-auth-code", form.Get("code"))
		assert.Equal(t, "cli-client", form.Get("client_id"))
		assert.NotEmpty(t, form.Get("redirect_uri"))
		verifier := form.Get("code_verifier")
		assert.GreaterOrEqual(t, len(verifier), 43)
	default:
		t.Fatal("token endpoint was never called")
	}
}

func TestLogin_ExtraParamsAppliedToTokenRequestAndPersisted(t *testing.T) {
	var capturedTokenURL *url.URL
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedTokenURL = &u
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-123","refresh_token":"rt-456","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	withStubbedBrowser(t, func(rawURL string) error {
		return simulateBrowserCallback(t, rawURL, nil, nil)
	})

	token, err := Login(context.Background(), LoginOptions{
		AuthURL:     "https://idp.example.com/oauth2/authorize",
		TokenURL:    tokenServer.URL,
		ClientID:    "cli-client",
		OpenBrowser: true,
		Timeout:     5 * time.Second,
		ExtraParams: map[string]string{"resource": "http://pb.openndx.local"},
	})

	assert.NoError(t, err)
	if assert.NotNil(t, capturedTokenURL) {
		// Thunder-style IDPs expect `resource` as a query param on /token, not just /authorize.
		assert.Equal(t, "http://pb.openndx.local", capturedTokenURL.Query().Get("resource"))
	}
	if assert.NotNil(t, token) {
		// It must also be persisted on the cached token so a later refresh (a
		// separate process invocation) can reuse it without being told again.
		assert.Equal(t, "http://pb.openndx.local", token.ExtraParams["resource"])
	}
}

func TestLogin_RepeatedCallbacksDoNotBlock(t *testing.T) {
	tokenServer := newTestTokenServer(t, nil, `{"access_token":"at-123","refresh_token":"rt-456","token_type":"Bearer","expires_in":3600}`, http.StatusOK)
	defer tokenServer.Close()

	withStubbedBrowser(t, func(rawURL string) error {
		// A flaky browser/IDP redelivering the redirect must not deadlock
		// the handler on the already-drained, capacity-1 resultCh - each of
		// these is a synchronous call, so a blocked handler here would hang
		// this stub (and therefore Login, which is still inside its
		// openBrowser call) forever rather than surfacing as a timeout.
		for i := 0; i < 3; i++ {
			if err := simulateBrowserCallback(t, rawURL, nil, nil); err != nil {
				return err
			}
		}
		return nil
	})

	done := make(chan error, 1)
	go func() {
		_, err := Login(context.Background(), LoginOptions{
			AuthURL:     "https://idp.example.com/oauth2/authorize",
			TokenURL:    tokenServer.URL,
			ClientID:    "cli-client",
			OpenBrowser: true,
			Timeout:     5 * time.Second,
		})
		done <- err
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Login did not return - a repeated callback likely deadlocked the handler")
	}
}

func TestLogin_StateMismatch(t *testing.T) {
	tokenServer := newTestTokenServer(t, nil, `{}`, http.StatusOK)
	defer tokenServer.Close()

	badState := "not-the-real-state"
	withStubbedBrowser(t, func(rawURL string) error {
		return simulateBrowserCallback(t, rawURL, nil, &badState)
	})

	_, err := Login(context.Background(), LoginOptions{
		AuthURL:     "https://idp.example.com/oauth2/authorize",
		TokenURL:    tokenServer.URL,
		ClientID:    "cli-client",
		OpenBrowser: true,
		Timeout:     5 * time.Second,
	})

	assert.Error(t, err)
	assert.ErrorContains(t, err, "state mismatch")
}

func TestLogin_AuthorizationDenied(t *testing.T) {
	tokenServer := newTestTokenServer(t, nil, `{}`, http.StatusOK)
	defer tokenServer.Close()

	withStubbedBrowser(t, func(rawURL string) error {
		u, err := url.Parse(rawURL)
		assert.NoError(t, err)
		redirectURI := u.Query().Get("redirect_uri")
		resp, err := http.Get(fmt.Sprintf("%s?error=access_denied&error_description=user+cancelled", redirectURI))
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		return nil
	})

	_, err := Login(context.Background(), LoginOptions{
		AuthURL:     "https://idp.example.com/oauth2/authorize",
		TokenURL:    tokenServer.URL,
		ClientID:    "cli-client",
		OpenBrowser: true,
		Timeout:     5 * time.Second,
	})

	assert.Error(t, err)
	assert.ErrorContains(t, err, "access_denied")
}

func TestLogin_TokenEndpointFailure(t *testing.T) {
	tokenServer := newTestTokenServer(t, nil, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	defer tokenServer.Close()

	withStubbedBrowser(t, func(rawURL string) error {
		return simulateBrowserCallback(t, rawURL, nil, nil)
	})

	_, err := Login(context.Background(), LoginOptions{
		AuthURL:     "https://idp.example.com/oauth2/authorize",
		TokenURL:    tokenServer.URL,
		ClientID:    "cli-client",
		OpenBrowser: true,
		Timeout:     5 * time.Second,
	})

	assert.Error(t, err)
	assert.ErrorContains(t, err, "status 400")
}

func TestLogin_Timeout(t *testing.T) {
	tokenServer := newTestTokenServer(t, nil, `{}`, http.StatusOK)
	defer tokenServer.Close()

	// Stub never hits the callback, simulating a user who never completes login.
	withStubbedBrowser(t, func(rawURL string) error { return nil })

	_, err := Login(context.Background(), LoginOptions{
		AuthURL:     "https://idp.example.com/oauth2/authorize",
		TokenURL:    tokenServer.URL,
		ClientID:    "cli-client",
		OpenBrowser: true,
		Timeout:     50 * time.Millisecond,
	})

	assert.Error(t, err)
	assert.ErrorContains(t, err, "timed out")
}

func TestRefreshToken_PreservesRefreshTokenWhenOmitted(t *testing.T) {
	formCh := make(chan url.Values, 1)
	var capturedTokenURL *url.URL
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		u := *r.URL
		capturedTokenURL = &u
		formCh <- r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	original := &Token{
		AccessToken:  "old-access",
		RefreshToken: "keep-me",
		TokenURL:     tokenServer.URL,
		ClientID:     "cli-client",
		ExpiresAt:    time.Now().Add(-time.Minute),
		ExtraParams:  map[string]string{"resource": "http://pb.openndx.local"},
	}

	refreshed, err := RefreshToken(context.Background(), nil, original)
	assert.NoError(t, err)
	if assert.NotNil(t, refreshed) {
		assert.Equal(t, "new-access", refreshed.AccessToken)
		// The IDP didn't return a new refresh token, so the old one must survive.
		assert.Equal(t, "keep-me", refreshed.RefreshToken)
		// The resource param must carry forward too, for the next refresh after this one.
		assert.Equal(t, "http://pb.openndx.local", refreshed.ExtraParams["resource"])
	}

	select {
	case form := <-formCh:
		assert.Equal(t, "refresh_token", form.Get("grant_type"))
		assert.Equal(t, "keep-me", form.Get("refresh_token"))
	default:
		t.Fatal("token endpoint was never called")
	}
	if assert.NotNil(t, capturedTokenURL) {
		assert.Equal(t, "http://pb.openndx.local", capturedTokenURL.Query().Get("resource"))
	}
}

func TestRefreshToken_NoRefreshTokenCached(t *testing.T) {
	_, err := RefreshToken(context.Background(), nil, &Token{AccessToken: "at"})
	assert.Error(t, err)
	assert.ErrorContains(t, err, "run login again")
}

func TestBuildAuthorizeURL_IncludesExtraParams(t *testing.T) {
	rawURL, err := buildAuthorizeURL(LoginOptions{
		AuthURL:  "https://idp.example.com/oauth2/authorize",
		ClientID: "cli-client",
		Scopes:   "openid profile",
		ExtraParams: map[string]string{
			"resource": "http://api.openndx.local",
		},
	}, "http://127.0.0.1:12345/callback", "state-123", "challenge-abc")

	assert.NoError(t, err)
	u, err := url.Parse(rawURL)
	assert.NoError(t, err)
	q := u.Query()
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "cli-client", q.Get("client_id"))
	assert.Equal(t, "http://127.0.0.1:12345/callback", q.Get("redirect_uri"))
	assert.Equal(t, "openid profile", q.Get("scope"))
	assert.Equal(t, "state-123", q.Get("state"))
	assert.Equal(t, "challenge-abc", q.Get("code_challenge"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, "http://api.openndx.local", q.Get("resource"))
}

func TestWithQueryParams(t *testing.T) {
	t.Run("adds params", func(t *testing.T) {
		out, err := withQueryParams("https://idp.example.com/oauth2/token", map[string]string{"resource": "http://pb.openndx.local"})
		assert.NoError(t, err)
		u, err := url.Parse(out)
		assert.NoError(t, err)
		assert.Equal(t, "http://pb.openndx.local", u.Query().Get("resource"))
	})

	t.Run("no params leaves URL untouched", func(t *testing.T) {
		out, err := withQueryParams("https://idp.example.com/oauth2/token", nil)
		assert.NoError(t, err)
		assert.Equal(t, "https://idp.example.com/oauth2/token", out)
	})

	t.Run("merges with existing query", func(t *testing.T) {
		out, err := withQueryParams("https://idp.example.com/oauth2/token?foo=bar", map[string]string{"resource": "http://pb.openndx.local"})
		assert.NoError(t, err)
		u, err := url.Parse(out)
		assert.NoError(t, err)
		assert.Equal(t, "bar", u.Query().Get("foo"))
		assert.Equal(t, "http://pb.openndx.local", u.Query().Get("resource"))
	})
}

// sanity check that the test JSON helper produces what the real IDP would.
func TestTokenResponseDecoding(t *testing.T) {
	var tr tokenResponse
	err := json.Unmarshal([]byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer","expires_in":60}`), &tr)
	assert.NoError(t, err)
	assert.Equal(t, "a", tr.AccessToken)
	assert.Equal(t, int64(60), tr.ExpiresIn)
}
