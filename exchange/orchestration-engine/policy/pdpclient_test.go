package policy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
)

func init() {
	logger.Init()
}

func TestNewPdpClient(t *testing.T) {
	baseUrl := "http://localhost:8080"
	client := NewPdpClient(baseUrl, nil)

	if client == nil {
		t.Fatal("Expected non-nil PdpClient")
	}

	if client.baseUrl != baseUrl {
		t.Errorf("Expected baseUrl %s, got %s", baseUrl, client.baseUrl)
	}

	if client.httpClient == nil {
		t.Error("Expected non-nil httpClient")
	}

	if client.httpClient.Timeout.Seconds() != 10 {
		t.Errorf("Expected timeout of 10 seconds, got %v", client.httpClient.Timeout)
	}
}

func TestMakePdpRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.URL.Path != "/api/v1/policy/decide" {
			t.Errorf("Expected path /api/v1/policy/decide, got %s", r.URL.Path)
		}

		response := PdpResponse{
			AppAuthorized:           true,
			AppRequiresOwnerConsent: false,
			ConsentRequiredFields:   []ConsentRequiredField{},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewPdpClient(server.URL, nil)

	request := &PdpRequest{
		AppId: "app456",
		RequiredFields: []RequiredField{
			{
				SchemaID:  "schema1",
				FieldName: "field1",
			},
		},
	}

	response, err := client.MakePdpRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response == nil {
		t.Fatal("Expected non-nil response")
	}

	if !response.AppAuthorized {
		t.Error("Expected AppAuthorized to be true")
	}

	if response.AppRequiresOwnerConsent {
		t.Error("Expected AppRequiresOwnerConsent to be false")
	}
}

func TestMakePdpRequest_ConsentRequired(t *testing.T) {
	displayName := "Sensitive Field"
	description := "Contains sensitive data"
	owner := OwnerCitizen

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := PdpResponse{
			AppAuthorized:           true,
			AppRequiresOwnerConsent: true,
			ConsentRequiredFields: []ConsentRequiredField{
				{
					FieldName:   "sensitiveField",
					SchemaID:    "schema1",
					DisplayName: &displayName,
					Description: &description,
					Owner:       &owner,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewPdpClient(server.URL, nil)

	request := &PdpRequest{
		AppId: "app456",
		RequiredFields: []RequiredField{
			{
				SchemaID:  "schema1",
				FieldName: "sensitiveField",
			},
		},
	}

	response, err := client.MakePdpRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !response.AppAuthorized {
		t.Error("Expected AppAuthorized to be true")
	}

	if !response.AppRequiresOwnerConsent {
		t.Error("Expected AppRequiresOwnerConsent to be true")
	}

	if len(response.ConsentRequiredFields) != 1 {
		t.Fatalf("Expected 1 consent required field, got %d", len(response.ConsentRequiredFields))
	}

	field := response.ConsentRequiredFields[0]
	if field.FieldName != "sensitiveField" {
		t.Errorf("Expected FieldName sensitiveField, got %s", field.FieldName)
	}

	if field.SchemaID != "schema1" {
		t.Errorf("Expected SchemaID schema1, got %s", field.SchemaID)
	}

	if field.DisplayName == nil || *field.DisplayName != displayName {
		t.Errorf("Expected DisplayName %s, got %v", displayName, field.DisplayName)
	}

	if field.Description == nil || *field.Description != description {
		t.Errorf("Expected Description %s, got %v", description, field.Description)
	}

	if field.Owner == nil || *field.Owner != owner {
		t.Errorf("Expected Owner %s, got %v", owner, field.Owner)
	}
}

func TestMakePdpRequest_AppNotAuthorized(t *testing.T) {
	unauthorizedField := ConsentRequiredField{
		FieldName: "restrictedField",
		SchemaID:  "schema1",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := PdpResponse{
			AppAuthorized:           false,
			UnauthorizedFields:      []ConsentRequiredField{unauthorizedField},
			AppRequiresOwnerConsent: false,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewPdpClient(server.URL, nil)

	request := &PdpRequest{
		AppId: "app456",
		RequiredFields: []RequiredField{
			{
				SchemaID:  "schema1",
				FieldName: "restrictedField",
			},
		},
	}

	response, err := client.MakePdpRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response.AppAuthorized {
		t.Error("Expected AppAuthorized to be false")
	}

	if len(response.UnauthorizedFields) != 1 {
		t.Fatalf("Expected 1 unauthorized field, got %d", len(response.UnauthorizedFields))
	}
}

func TestMakePdpRequest_AppAccessExpired(t *testing.T) {
	expiredField := ConsentRequiredField{
		FieldName: "expiredField",
		SchemaID:  "schema1",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := PdpResponse{
			AppAuthorized:    true,
			AppAccessExpired: true,
			ExpiredFields:    []ConsentRequiredField{expiredField},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewPdpClient(server.URL, nil)

	request := &PdpRequest{
		AppId: "app456",
		RequiredFields: []RequiredField{
			{
				SchemaID:  "schema1",
				FieldName: "expiredField",
			},
		},
	}

	response, err := client.MakePdpRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !response.AppAccessExpired {
		t.Error("Expected AppAccessExpired to be true")
	}

	if len(response.ExpiredFields) != 1 {
		t.Fatalf("Expected 1 expired field, got %d", len(response.ExpiredFields))
	}
}

func TestMakePdpRequest_NetworkError(t *testing.T) {
	client := NewPdpClient("http://invalid-url-that-does-not-exist:9999", nil)

	request := &PdpRequest{
		AppId: "app456",
		RequiredFields: []RequiredField{
			{
				SchemaID:  "schema1",
				FieldName: "field1",
			},
		},
	}

	response, err := client.MakePdpRequest(context.Background(), request)

	if err == nil {
		t.Error("Expected error when network request fails")
	}

	if response != nil {
		t.Errorf("Expected nil response on error, got %v", response)
	}
}

func TestMakePdpRequest_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewPdpClient(server.URL, nil)

	request := &PdpRequest{
		AppId: "app456",
		RequiredFields: []RequiredField{
			{
				SchemaID:  "schema1",
				FieldName: "field1",
			},
		},
	}

	response, err := client.MakePdpRequest(context.Background(), request)

	if err == nil {
		t.Error("Expected error when server returns invalid JSON")
	}

	if response != nil {
		t.Errorf("Expected nil response on error, got %v", response)
	}
}

func TestMakePdpRequest_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	client := NewPdpClient(server.URL, nil)

	request := &PdpRequest{
		AppId: "app456",
		RequiredFields: []RequiredField{
			{
				SchemaID:  "schema1",
				FieldName: "field1",
			},
		},
	}

	response, err := client.MakePdpRequest(context.Background(), request)

	if err == nil {
		t.Error("Expected error when server returns non-200 status code")
	}

	if response != nil {
		t.Errorf("Expected nil response on error, got %v", response)
	}
}

func TestMakePdpRequest_BadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid request parameters"}`))
	}))
	defer server.Close()

	client := NewPdpClient(server.URL, nil)

	request := &PdpRequest{
		AppId: "app456",
		RequiredFields: []RequiredField{
			{
				SchemaID:  "schema1",
				FieldName: "field1",
			},
		},
	}

	response, err := client.MakePdpRequest(context.Background(), request)

	if err == nil {
		t.Error("Expected error when server returns 400 status code")
	}

	if response != nil {
		t.Errorf("Expected nil response on error, got %v", response)
	}
}
