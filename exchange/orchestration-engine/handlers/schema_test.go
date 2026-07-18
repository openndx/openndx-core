package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/services"
	"github.com/stretchr/testify/assert"
)

func init() {
	logger.Init()
}

func TestNewSchemaHandler(t *testing.T) {
	handler := NewSchemaHandler(nil)
	assert.NotNil(t, handler)
	assert.Nil(t, handler.schemaService)
}

func TestSchemaHandler_CreateSchema_InvalidJSON_ReturnsBadRequest(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{})

	req := httptest.NewRequest(http.MethodPost, "/sdl", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateSchema(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid JSON")
}

func TestSchemaHandler_CreateSchema_MissingFields_ReturnsBadRequest(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{})

	reqBody := CreateSchemaRequest{
		Version: "1.0.0",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/sdl", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateSchema(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "SDL and created_by are required")
}

func TestSchemaHandler_CreateSchema_NoService(t *testing.T) {
	handler := NewSchemaHandler(nil)

	reqBody := CreateSchemaRequest{
		Version:   "1.0.0",
		SDL:       "type Query { test: String }",
		CreatedBy: "test-user",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/sdl", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateSchema(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "database not connected")
}

func TestSchemaHandler_GetSchemas_NoService(t *testing.T) {
	handler := NewSchemaHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/sdl/versions", nil)
	w := httptest.NewRecorder()

	handler.GetSchemas(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSchemaHandler_GetSchemas_Success(t *testing.T) {
	mockService := &mockSchemaService{
		getAllSchemasFn: func() ([]services.Schema, error) {
			return []services.Schema{
				{Version: "1.0.0", SDL: "type Query { test: String }"},
			}, nil
		},
	}
	handler := NewSchemaHandler(mockService)

	req := httptest.NewRequest(http.MethodGet, "/sdl/versions", nil)
	w := httptest.NewRecorder()

	handler.GetSchemas(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "test: String")
}

func TestSchemaHandler_GetActiveSchema_NoService(t *testing.T) {
	handler := NewSchemaHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/sdl", nil)
	w := httptest.NewRecorder()

	handler.GetActiveSchema(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSchemaHandler_GetActiveSchema_Success(t *testing.T) {
	mockService := &mockSchemaService{
		getActiveSchemaFn: func() (*services.Schema, error) {
			return &services.Schema{SDL: "type Query { test: String }"}, nil
		},
	}
	handler := NewSchemaHandler(mockService)

	req := httptest.NewRequest(http.MethodGet, "/sdl", nil)
	w := httptest.NewRecorder()

	handler.GetActiveSchema(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "test: String")
}

func TestSchemaHandler_ActivateSchema_NoService(t *testing.T) {
	handler := NewSchemaHandler(nil)

	req := httptest.NewRequest(http.MethodPost, "/sdl/versions/1.0.0/activate", nil)
	req.SetPathValue("version", "1.0.0")
	w := httptest.NewRecorder()

	handler.ActivateSchema(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSchemaHandler_ActivateSchema_Success(t *testing.T) {
	called := false
	mockService := &mockSchemaService{
		activateSchemaFn: func(version string) error {
			assert.Equal(t, "1.0.0", version)
			called = true
			return nil
		},
	}
	handler := NewSchemaHandler(mockService)

	req := httptest.NewRequest(http.MethodPost, "/sdl/versions/1.0.0/activate", nil)
	req.SetPathValue("version", "1.0.0")
	w := httptest.NewRecorder()

	handler.ActivateSchema(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}

func TestSchemaHandler_ValidateSDL_InvalidJSON(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{})

	req := httptest.NewRequest(http.MethodPost, "/sdl/validate", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	handler.ValidateSDL(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid JSON")
}

func TestSchemaHandler_ValidateSDL_NoService(t *testing.T) {
	handler := NewSchemaHandler(nil)

	reqBody := ValidateSDLRequest{
		SDL: "type Query { test: String }",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/sdl/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ValidateSDL(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSchemaHandler_ValidateSDL_Success(t *testing.T) {
	mockService := &mockSchemaService{
		validateSDLFn: func(s string) bool {
			assert.Equal(t, "type Query { test: String }", s)
			return true
		},
	}
	handler := NewSchemaHandler(mockService)

	reqBody := ValidateSDLRequest{
		SDL: "type Query { test: String }",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/sdl/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ValidateSDL(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "true")
}

func TestSchemaHandler_CheckCompatibility_InvalidJSON(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{})

	req := httptest.NewRequest(http.MethodPost, "/sdl/check-compatibility", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	handler.CheckCompatibility(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid JSON")
}

func TestSchemaHandler_CheckCompatibility_NoService(t *testing.T) {
	handler := NewSchemaHandler(nil)

	reqBody := ValidateSDLRequest{
		SDL: "type Query { test: String }",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/sdl/check-compatibility", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CheckCompatibility(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSchemaHandler_CheckCompatibility_Success(t *testing.T) {
	mockService := &mockSchemaService{
		checkCompatibilityFn: func(s string) (bool, string) {
			assert.Equal(t, "type Query { test: String }", s)
			return true, "ok"
		},
	}
	handler := NewSchemaHandler(mockService)

	reqBody := ValidateSDLRequest{
		SDL: "type Query { test: String }",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/sdl/check-compatibility", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CheckCompatibility(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"compatible\":true")
}

type mockSchemaService struct {
	createSchemaFn       func(version, sdl, createdBy string) (*services.Schema, error)
	getAllSchemasFn      func() ([]services.Schema, error)
	getActiveSchemaFn    func() (*services.Schema, error)
	activateSchemaFn     func(version string) error
	validateSDLFn        func(sdl string) bool
	checkCompatibilityFn func(newSDL string) (bool, string)
}

func (m *mockSchemaService) CreateSchema(version, sdl, createdBy string) (*services.Schema, error) {
	if m.createSchemaFn != nil {
		return m.createSchemaFn(version, sdl, createdBy)
	}
	return nil, nil
}

func (m *mockSchemaService) GetAllSchemas() ([]services.Schema, error) {
	if m.getAllSchemasFn != nil {
		return m.getAllSchemasFn()
	}
	return nil, nil
}

func (m *mockSchemaService) GetActiveSchema() (*services.Schema, error) {
	if m.getActiveSchemaFn != nil {
		return m.getActiveSchemaFn()
	}
	return nil, errors.New("not implemented")
}

func (m *mockSchemaService) ActivateSchema(version string) error {
	if m.activateSchemaFn != nil {
		return m.activateSchemaFn(version)
	}
	return nil
}

func (m *mockSchemaService) ValidateSDL(sdl string) bool {
	if m.validateSDLFn != nil {
		return m.validateSDLFn(sdl)
	}
	return false
}

func (m *mockSchemaService) CheckCompatibility(newSDL string) (bool, string) {
	if m.checkCompatibilityFn != nil {
		return m.checkCompatibilityFn(newSDL)
	}
	return false, ""
}
