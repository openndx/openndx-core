package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/services"
	"github.com/go-chi/chi/v5"
)

// SchemaService defines the behavior SchemaHandler depends on.
type SchemaService interface {
	CreateSchema(version, sdl, createdBy string) (*services.Schema, error)
	GetAllSchemas() ([]services.Schema, error)
	GetActiveSchema() (*services.Schema, error)
	ActivateSchema(version string) error
	ValidateSDL(sdl string) bool
	CheckCompatibility(newSDL string) (bool, string)
}

// SchemaHandler handles HTTP requests for schema management
type SchemaHandler struct {
	schemaService SchemaService
}

// NewSchemaHandler creates a new schema handler
func NewSchemaHandler(schemaService SchemaService) *SchemaHandler {
	return &SchemaHandler{
		schemaService: schemaService,
	}
}

// CreateSchemaRequest represents a request to create a new schema
type CreateSchemaRequest struct {
	Version   string `json:"version"`
	SDL       string `json:"sdl"`
	CreatedBy string `json:"created_by"`
}

// ValidateSDLRequest represents a request to validate SDL
type ValidateSDLRequest struct {
	SDL string `json:"sdl"`
}

// CreateSchema handles POST /sdl - create a new schema version
func (h *SchemaHandler) CreateSchema(w http.ResponseWriter, r *http.Request) {
	if h.schemaService == nil {
		http.Error(w, "Schema management not available - database not connected", http.StatusServiceUnavailable)
		return
	}

	var req CreateSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.SDL == "" || req.CreatedBy == "" {
		http.Error(w, "SDL and created_by are required", http.StatusBadRequest)
		return
	}

	if req.Version == "" {
		req.Version = "1.0.0" // Default version
	}

	schema, err := h.schemaService.CreateSchema(req.Version, req.SDL, req.CreatedBy)
	if err != nil {
		logger.Log.Error("Failed to create schema", "error", err, "version", req.Version)
		// Return generic error to avoid exposing internal details
		http.Error(w, "Failed to create schema", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schema)
}

// GetSchemas handles GET /sdl/versions - get all schema versions
func (h *SchemaHandler) GetSchemas(w http.ResponseWriter, r *http.Request) {
	if h.schemaService == nil {
		http.Error(w, "Schema management not available - database not connected", http.StatusServiceUnavailable)
		return
	}

	schemas, err := h.schemaService.GetAllSchemas()
	if err != nil {
		logger.Log.Error("Failed to get schemas", "error", err)
		// Log detailed error but return generic message to client
		// Avoid exposing database structure or query details
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schemas)
}

// GetActiveSchema handles GET /sdl - get the active schema
func (h *SchemaHandler) GetActiveSchema(w http.ResponseWriter, r *http.Request) {
	if h.schemaService == nil {
		http.Error(w, "Schema management not available - database not connected", http.StatusServiceUnavailable)
		return
	}

	schema, err := h.schemaService.GetActiveSchema()
	if err != nil {
		logger.Log.Error("Failed to get active schema", "error", err)
		// Log detailed error but return generic message to client
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if schema == nil {
		http.Error(w, "No active schema found", http.StatusNotFound)
		return
	}

	response := map[string]string{"sdl": schema.SDL}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ActivateSchema handles POST /sdl/versions/{version}/activate - activate a schema version
func (h *SchemaHandler) ActivateSchema(w http.ResponseWriter, r *http.Request) {
	if h.schemaService == nil {
		http.Error(w, "Schema management not available - database not connected", http.StatusServiceUnavailable)
		return
	}

	// Extract version from URL path (simplified)
	version := chi.URLParam(r, "version")

	err := h.schemaService.ActivateSchema(version)
	if err != nil {
		logger.Log.Error("Failed to activate schema", "error", err, "version", version)
		// Return generic error to avoid exposing internal details
		http.Error(w, "Schema not found or cannot be activated", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Schema activated successfully"})
}

// ValidateSDL handles POST /sdl/validate - validate SDL syntax
func (h *SchemaHandler) ValidateSDL(w http.ResponseWriter, r *http.Request) {
	if h.schemaService == nil {
		http.Error(w, "Schema management not available - database not connected", http.StatusServiceUnavailable)
		return
	}

	var req ValidateSDLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	valid := h.schemaService.ValidateSDL(req.SDL)
	response := map[string]bool{"valid": valid}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CheckCompatibility handles POST /sdl/check-compatibility - check backward compatibility
func (h *SchemaHandler) CheckCompatibility(w http.ResponseWriter, r *http.Request) {
	if h.schemaService == nil {
		http.Error(w, "Schema management not available - database not connected", http.StatusServiceUnavailable)
		return
	}

	var req ValidateSDLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	compatible, reason := h.schemaService.CheckCompatibility(req.SDL)
	response := map[string]interface{}{
		"compatible": compatible,
		"reason":     reason,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
