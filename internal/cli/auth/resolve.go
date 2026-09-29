package auth

import (
	"context"
	"fmt"
	"net/http"
)

// EnsureFreshToken loads the cached token from path and, if it has expired,
// refreshes it and persists the refreshed token back to the same path.
func EnsureFreshToken(ctx context.Context, path string, httpClient *http.Client) (*Token, error) {
	token, err := LoadToken(path)
	if err != nil {
		return nil, fmt.Errorf("not logged in (run 'ondx login' first): %w", err)
	}

	if !token.Expired() {
		return token, nil
	}

	refreshed, err := RefreshToken(ctx, httpClient, token)
	if err != nil {
		return nil, fmt.Errorf("session expired and could not be refreshed (run 'ondx login' again): %w", err)
	}

	if err := SaveToken(path, refreshed); err != nil {
		// Non-fatal: we still have a usable in-memory token for this run.
		fmt.Printf("Warning: failed to persist refreshed token: %v\n", err)
	}

	return refreshed, nil
}
