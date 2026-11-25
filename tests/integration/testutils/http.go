// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package testutils

import (
	"fmt"
	"net/http"
	"time"
)

const (
	DefaultRetryAttempts       = 30
	DefaultRetryInterval       = 1 * time.Second
	DefaultServiceCheckTimeout = 2 * time.Second
)

// WaitForService waits for a service to become available by sending a request to the given URL.
// It retries for a specified number of times with a delay between attempts.
// Logs progress every 5 attempts to reduce noise.
func WaitForService(url string, maxAttempts int) error {
	if maxAttempts <= 0 {
		maxAttempts = DefaultRetryAttempts
	}

	client := &http.Client{
		Timeout: DefaultServiceCheckTimeout,
	}

	for i := 0; i < maxAttempts; i++ {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		// Only log every 5 attempts to reduce noise
		if i > 0 && i%5 == 0 {
			fmt.Printf("  Still waiting for %s... (attempt %d/%d)\n", url, i+1, maxAttempts)
		}
		time.Sleep(DefaultRetryInterval)
	}

	return fmt.Errorf("service at %s did not become available after %d attempts", url, maxAttempts)
}
