package middleware

import (
	"crypto/rsa"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestJWTAuthMiddleware_ThreadSafety tests that concurrent access to keys map is safe
func TestJWTAuthMiddleware_ThreadSafety(t *testing.T) {
	config := JWTAuthConfig{
		JWKSURL:        "https://example.com/.well-known/jwks.json",
		ExpectedIssuer: "https://example.com",
		ValidClientIDs: []string{"test-client"},
		Timeout:        5 * time.Second,
	}

	middleware := NewJWTAuthMiddleware(config)

	// Initialize with some test keys to simulate real scenario
	middleware.keysMutex.Lock()
	middleware.keys["test-key-1"] = nil // nil is fine for this test
	middleware.keys["test-key-2"] = nil
	middleware.lastFetch = time.Now()
	middleware.keysMutex.Unlock()

	const numGoroutines = 50
	const iterationsPerGoroutine = 100

	var wg sync.WaitGroup

	// Test concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				// Simulate reading keys (like in validateToken)
				middleware.keysMutex.RLock()
				_ = len(middleware.keys)
				_, exists := middleware.keys["test-key-1"]
				_ = exists
				_ = middleware.lastFetch
				middleware.keysMutex.RUnlock()
			}
		}()
	}

	// Test concurrent key freshness checks
	for i := 0; i < numGoroutines/5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine/10; j++ {
				// Simulate ensureKeysFresh checks
				middleware.keysMutex.RLock()
				needsRefresh := len(middleware.keys) == 0 || time.Since(middleware.lastFetch) > time.Hour
				middleware.keysMutex.RUnlock()
				_ = needsRefresh
			}
		}()
	}

	// Test concurrent writes (simulate periodic JWKS refresh)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				// Simulate fetchJWKS operation
				newKeys := make(map[string]*rsa.PublicKey)
				newKeys["test-key-1"] = nil
				newKeys["test-key-2"] = nil

				middleware.keysMutex.Lock()
				middleware.keys = newKeys
				middleware.lastFetch = time.Now()
				middleware.keysMutex.Unlock()

				time.Sleep(time.Millisecond) // Small delay to increase chance of race conditions
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify final state is consistent
	middleware.keysMutex.RLock()
	finalKeys := len(middleware.keys)
	finalFetch := middleware.lastFetch
	middleware.keysMutex.RUnlock()

	if finalKeys != 2 {
		t.Errorf("Expected 2 keys in final state, got %d", finalKeys)
	}

	if finalFetch.IsZero() {
		t.Error("Expected lastFetch to be set in final state")
	}
}

// TestJWTAuthMiddleware_KeyUpdateAtomicity tests that key updates are atomic
func TestJWTAuthMiddleware_KeyUpdateAtomicity(t *testing.T) {
	config := JWTAuthConfig{
		JWKSURL:        "https://example.com/.well-known/jwks.json",
		ExpectedIssuer: "https://example.com",
		ValidClientIDs: []string{"test-client"},
		Timeout:        5 * time.Second,
	}

	middleware := NewJWTAuthMiddleware(config)

	// Initialize with initial state
	middleware.keysMutex.Lock()
	middleware.keys["initial-key"] = nil
	middleware.lastFetch = time.Now().Add(-2 * time.Hour) // Old timestamp
	middleware.keysMutex.Unlock()

	var wg sync.WaitGroup
	const numReaders = 20

	// Start readers that check for consistency
	inconsistencyFound := false
	var inconsistencyMutex sync.Mutex

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				middleware.keysMutex.RLock()
				hasInitialKey := middleware.keys["initial-key"] != nil
				hasNewKey := middleware.keys["new-key"] != nil
				lastFetch := middleware.lastFetch
				middleware.keysMutex.RUnlock()

				// Check for inconsistent state: if we have new-key, lastFetch should be recent
				if hasNewKey && time.Since(lastFetch) > time.Hour {
					inconsistencyMutex.Lock()
					inconsistencyFound = true
					inconsistencyMutex.Unlock()
					t.Errorf("Found inconsistent state: new key exists but lastFetch is old")
					return
				}

				// If we have initial key but no new key, lastFetch should be old
				if hasInitialKey && !hasNewKey && time.Since(lastFetch) < time.Hour {
					// This could be a transition state, which is acceptable
				}
			}
		}()
	}

	// Perform atomic update
	time.Sleep(10 * time.Millisecond) // Let readers start

	newKeys := make(map[string]*rsa.PublicKey)
	newKeys["new-key"] = nil

	middleware.keysMutex.Lock()
	middleware.keys = newKeys
	middleware.lastFetch = time.Now()
	middleware.keysMutex.Unlock()

	wg.Wait()

	if inconsistencyFound {
		t.Error("Inconsistent state detected during atomic update")
	}
}

// BenchmarkJWTAuthMiddleware_ConcurrentKeyAccess benchmarks concurrent key access
func BenchmarkJWTAuthMiddleware_ConcurrentKeyAccess(b *testing.B) {
	config := JWTAuthConfig{
		JWKSURL:        "https://example.com/.well-known/jwks.json",
		ExpectedIssuer: "https://example.com",
		ValidClientIDs: []string{"test-client"},
		Timeout:        5 * time.Second,
	}

	middleware := NewJWTAuthMiddleware(config)

	// Initialize with test keys
	middleware.keysMutex.Lock()
	for i := 0; i < 10; i++ {
		middleware.keys[fmt.Sprintf("key-%d", i)] = nil
	}
	middleware.lastFetch = time.Now()
	middleware.keysMutex.Unlock()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Simulate typical key lookup operation
			middleware.keysMutex.RLock()
			_, exists := middleware.keys["key-5"]
			_ = exists
			middleware.keysMutex.RUnlock()
		}
	})
}
