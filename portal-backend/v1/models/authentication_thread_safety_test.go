package models

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestAuthenticatedUser_MemberIDCaching_ThreadSafety tests the thread safety of member ID caching
func TestAuthenticatedUser_MemberIDCaching_ThreadSafety(t *testing.T) {
	// Create a test user
	claims := &UserClaims{
		Email:     "threadtest@example.com",
		IdpUserID: "thread-test-123",
		Roles:     FlexibleStringSlice{"OpenDIF_Member"},
	}

	user, err := NewAuthenticatedUser(claims)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	const numGoroutines = 100
	const iterations = 10

	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines*iterations)

	// Test concurrent access to member ID caching
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				memberID := fmt.Sprintf("member-%d-%d", routineID, j)

				// Simulate concurrent set operations
				if j%2 == 0 {
					user.SetCachedMemberID(memberID, nil)
				} else {
					user.SetCachedMemberID("", fmt.Errorf("error-%d-%d", routineID, j))
				}

				// Simulate concurrent read operations
				// Read both values atomically to check consistency
				cachedID, hasCached, cachedErr := user.GetCachedMemberIDWithError()

				// Verify consistency: if there's a cached ID, there should be no error
				// and if there's an error, there should be no cached ID
				if hasCached && cachedID != "" && cachedErr != nil {
					errorChan <- fmt.Errorf("inconsistent state: have memberID '%s' but also error '%v'", cachedID, cachedErr)
					return
				}

				// Small delay to increase chance of race conditions if not properly synchronized
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errorChan)

	// Check for any errors
	for err := range errorChan {
		t.Errorf("Thread safety violation: %v", err)
	}

	// Final state should be consistent
	finalID, hasFinalID, finalErr := user.GetCachedMemberIDWithError()

	if hasFinalID && finalID != "" && finalErr != nil {
		t.Errorf("Final state is inconsistent: memberID='%s', error='%v'", finalID, finalErr)
	}

	t.Logf("Thread safety test completed successfully with %d goroutines and %d iterations each", numGoroutines, iterations)
}

// TestAuthenticatedUser_MemberIDCaching_BasicFunctionality tests basic member ID caching functionality
func TestAuthenticatedUser_MemberIDCaching_BasicFunctionality(t *testing.T) {
	// Create a test user
	claims := &UserClaims{
		Email:     "cachetest@example.com",
		IdpUserID: "cache-test-123",
		Roles:     FlexibleStringSlice{"OpenDIF_Member"},
	}

	user, err := NewAuthenticatedUser(claims)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Initially, no member ID should be cached
	memberID, cached := user.GetCachedMemberID()
	if cached {
		t.Errorf("Expected no cached member ID initially, but got: %s", memberID)
	}
	if memberID != "" {
		t.Errorf("Expected empty member ID initially, but got: %s", memberID)
	}

	cachedErr := user.GetCachedMemberIDError()
	if cachedErr != nil {
		t.Errorf("Expected no cached error initially, but got: %v", cachedErr)
	}

	// Test caching a successful result
	testMemberID := "test-member-123"
	user.SetCachedMemberID(testMemberID, nil)

	memberID, cached = user.GetCachedMemberID()
	if !cached {
		t.Errorf("Expected cached member ID after setting, but got cached=false")
	}
	if memberID != testMemberID {
		t.Errorf("Expected member ID '%s', but got: '%s'", testMemberID, memberID)
	}

	cachedErr = user.GetCachedMemberIDError()
	if cachedErr != nil {
		t.Errorf("Expected no cached error after successful cache, but got: %v", cachedErr)
	}

	// Test caching an error result
	testError := fmt.Errorf("test error")
	user.SetCachedMemberID("", testError)

	memberID, cached = user.GetCachedMemberID()
	if cached {
		t.Errorf("Expected no cached member ID after error, but got cached=true with ID: %s", memberID)
	}
	if memberID != "" {
		t.Errorf("Expected empty member ID after error, but got: %s", memberID)
	}

	cachedErr = user.GetCachedMemberIDError()
	if cachedErr == nil {
		t.Errorf("Expected cached error, but got nil")
	}
	if cachedErr.Error() != testError.Error() {
		t.Errorf("Expected error '%v', but got: '%v'", testError, cachedErr)
	}
}
