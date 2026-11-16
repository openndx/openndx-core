package integration_test

import (
	"encoding/json"
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

func TestPortalBackend_Readiness(t *testing.T) {
	resp, err := http.Get(portalBackendURL + "/health/readiness")
	if err != nil {
		t.Skipf("Portal Backend not available (expected in CI mode): %v", err)
		return
	}
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}
