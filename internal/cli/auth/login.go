package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LoginOptions configures a browser-based OAuth2 Authorization Code + PKCE login.
type LoginOptions struct {
	AuthURL     string
	TokenURL    string
	ClientID    string
	Scopes      string
	ExtraParams map[string]string // IDP-specific extras, e.g. "resource" or "audience"
	HTTPClient  *http.Client
	OpenBrowser bool
	Timeout     time.Duration
	// CallbackPort pins the local redirect listener to a specific loopback
	// port, for identity providers (like ThunderID today) whose redirectUris
	// allow-list requires an exact match rather than a wildcard/any port. 0
	// means let the OS assign a free ephemeral port (the RFC 8252-preferred
	// default, used whenever the IDP supports it).
	CallbackPort int
}

// openBrowser launches the system's default browser at the given URL. It is
// a variable so tests can stub it out without actually opening a browser.
var openBrowser = defaultOpenBrowser

func defaultOpenBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

type callbackResult struct {
	code string
	err  error
}

// Login runs a browser-based OAuth2 Authorization Code + PKCE flow and returns
// the resulting token. Per RFC 8252 (OAuth for native apps), it starts a
// short-lived local HTTP server on a loopback, OS-assigned port to receive
// the redirect, opens the system browser to the authorization URL, waits for
// the callback, then exchanges the code for a token.
func Login(ctx context.Context, opts LoginOptions) (*Token, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}

	pkce, err := NewPKCE()
	if err != nil {
		return nil, err
	}
	state, err := NewState()
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opts.CallbackPort))
	if err != nil {
		return nil, fmt.Errorf("failed to start local callback listener on port %d: %w", opts.CallbackPort, err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// resultCh has room for exactly one result, since Login only ever reads
	// one; callbackOnce keeps a second (e.g. duplicate or retried) callback
	// hit from blocking on that full channel forever - srv.Shutdown doesn't
	// interrupt an in-flight handler, so a blocked send would otherwise wait
	// out its whole shutdown deadline.
	resultCh := make(chan callbackResult, 1)
	var callbackOnce sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var result callbackResult
		switch {
		case q.Get("error") != "":
			result = callbackResult{err: fmt.Errorf("authorization failed: %s: %s", q.Get("error"), q.Get("error_description"))}
			writeCallbackResponse(w, false)
		case q.Get("state") != state:
			result = callbackResult{err: fmt.Errorf("state mismatch in callback: possible CSRF, aborting login")}
			writeCallbackResponse(w, false)
		case q.Get("code") == "":
			result = callbackResult{err: fmt.Errorf("no authorization code returned by identity provider")}
			writeCallbackResponse(w, false)
		default:
			result = callbackResult{code: q.Get("code")}
			writeCallbackResponse(w, true)
		}
		callbackOnce.Do(func() { resultCh <- result })
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	authorizeURL, err := buildAuthorizeURL(opts, redirectURI, state, pkce.Challenge)
	if err != nil {
		return nil, err
	}

	if opts.OpenBrowser {
		if err := openBrowser(authorizeURL); err != nil {
			fmt.Printf("Could not open a browser automatically (%v).\nPlease open this URL manually:\n%s\n\n", err, authorizeURL)
		} else {
			fmt.Printf("Opening your browser to log in. If it doesn't open automatically, visit:\n%s\n\n", authorizeURL)
		}
	} else {
		fmt.Printf("Open this URL to log in:\n%s\n\n", authorizeURL)
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}
		return exchangeCode(ctx, opts, res.code, redirectURI, pkce.Verifier)
	case <-time.After(opts.Timeout):
		return nil, fmt.Errorf("login timed out after %s waiting for browser callback", opts.Timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func buildAuthorizeURL(opts LoginOptions, redirectURI, state, challenge string) (string, error) {
	u, err := url.Parse(opts.AuthURL)
	if err != nil {
		return "", fmt.Errorf("invalid authorization URL: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", opts.ClientID)
	q.Set("redirect_uri", redirectURI)
	if opts.Scopes != "" {
		q.Set("scope", opts.Scopes)
	}
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	for k, v := range opts.ExtraParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// withQueryParams returns rawURL with params merged into its query string,
// added on top of (and overriding on key collision) whatever query the URL
// already carries.
func withQueryParams(rawURL string, params map[string]string) (string, error) {
	if len(params) == 0 {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func writeCallbackResponse(w http.ResponseWriter, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	const bodyStyle = "display:flex;flex-direction:column;align-items:center;justify-content:center;" +
		"height:100vh;margin:0;text-align:center;font-family:sans-serif"
	if ok {
		// window.close() only works on a tab the browser considers
		// script-opened; since this tab was opened by the OS's "open URL"
		// command instead, some browsers will ignore it and leave the tab
		// open - hence the fallback text still being shown underneath.
		_, _ = io.WriteString(w, `<html><body style="`+bodyStyle+`">
<h3>Login successful.</h3>
<p>You may close this window and return to the terminal.</p>
<script>window.close();</script>
</body></html>`)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, `<html><body style="`+bodyStyle+`">
<h3>Login failed.</h3>
<p>You may close this window and return to the terminal.</p>
</body></html>`)
}

// tokenResponse mirrors the standard OAuth2 token endpoint JSON response.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

func exchangeCode(ctx context.Context, opts LoginOptions, code, redirectURI, verifier string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", opts.ClientID)
	form.Set("code_verifier", verifier)

	return doTokenRequest(ctx, opts, form)
}

// RefreshToken exchanges a refresh token for a new access token, using the
// token endpoint and client ID recorded on the token itself.
func RefreshToken(ctx context.Context, httpClient *http.Client, token *Token) (*Token, error) {
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("cached token has no refresh token; run login again")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", token.RefreshToken)
	form.Set("client_id", token.ClientID)

	newToken, err := doTokenRequest(ctx, LoginOptions{
		TokenURL:    token.TokenURL,
		ClientID:    token.ClientID,
		HTTPClient:  httpClient,
		ExtraParams: token.ExtraParams, // e.g. ThunderID's resource=... requirement, remembered from login
	}, form)
	if err != nil {
		return nil, err
	}
	// Not every IDP rotates refresh tokens on use; if the response omitted a
	// new one, keep using the one we already have instead of losing it.
	if newToken.RefreshToken == "" {
		newToken.RefreshToken = token.RefreshToken
	}
	return newToken, nil
}

func doTokenRequest(ctx context.Context, opts LoginOptions, form url.Values) (*Token, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}

	// Some IDPs (e.g. ThunderID, which binds an access token's audience to
	// whatever resource server is requested) need extras like resource=...
	// on the /token call too, not just the /authorize call.
	tokenURL, err := withQueryParams(opts.TokenURL, opts.ExtraParams)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach token endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint response did not include an access token")
	}

	tokenType := tr.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	expiresIn := tr.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}

	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tokenType,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		TokenURL:     opts.TokenURL,
		ClientID:     opts.ClientID,
		ExtraParams:  opts.ExtraParams,
	}, nil
}
