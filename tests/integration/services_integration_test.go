// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package integration_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortalBackend_Health(t *testing.T) {
	// Portal Backend may not be running in all test environments (e.g., CI mode)
	// Skip test if service is not available
	resp, err := http.Get(portalBackendURL + "/health")
	if err != nil {
		t.Skipf("Portal Backend not available (expected in CI mode): %v", err)
		return
	}
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
