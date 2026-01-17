package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestConsentField_Equals(t *testing.T) {
	field1 := ConsentField{
		FieldName: "email",
		SchemaID:  "schema-1",
		Owner:     OwnerCitizen,
	}

	field2 := ConsentField{
		FieldName: "email",
		SchemaID:  "schema-1",
		Owner:     OwnerCitizen,
	}

	assert.True(t, field1.Equals(field2))
}

func TestConsentField_Equals_DifferentFieldName(t *testing.T) {
	field1 := ConsentField{
		FieldName: "email",
		SchemaID:  "schema-1",
		Owner:     OwnerCitizen,
	}

	field2 := ConsentField{
		FieldName: "name",
		SchemaID:  "schema-1",
		Owner:     OwnerCitizen,
	}

	assert.False(t, field1.Equals(field2))
}

func TestConsentField_Equals_WithDisplayName(t *testing.T) {
	displayName := "Email Address"
	field1 := ConsentField{
		FieldName:   "email",
		SchemaID:    "schema-1",
		DisplayName: &displayName,
		Owner:       OwnerCitizen,
	}

	field2 := ConsentField{
		FieldName:   "email",
		SchemaID:    "schema-1",
		DisplayName: &displayName,
		Owner:       OwnerCitizen,
	}

	assert.True(t, field1.Equals(field2))
}

func TestConsentField_Equals_OneWithDisplayName(t *testing.T) {
	displayName := "Email Address"
	field1 := ConsentField{
		FieldName:   "email",
		SchemaID:    "schema-1",
		DisplayName: &displayName,
		Owner:       OwnerCitizen,
	}

	field2 := ConsentField{
		FieldName: "email",
		SchemaID:  "schema-1",
		Owner:     OwnerCitizen,
	}

	assert.False(t, field1.Equals(field2))
}

func TestConsentRecord_TableName(t *testing.T) {
	record := ConsentRecord{}
	assert.Equal(t, "consent_records", record.TableName())
}

func TestConsentRecord_ToConsentResponseInternalView_Pending(t *testing.T) {
	portalURL := "http://portal.example.com/consent/123"
	record := ConsentRecord{
		ConsentID:        uuid.New(),
		Status:           string(StatusPending),
		ConsentPortalURL: portalURL,
		Fields:           []ConsentField{{FieldName: "email", SchemaID: "schema-1", Owner: OwnerCitizen}},
	}

	response := record.ToConsentResponseInternalView()

	assert.Equal(t, record.ConsentID.String(), response.ConsentID)
	assert.Equal(t, string(StatusPending), response.Status)
	assert.NotNil(t, response.ConsentPortalURL)
	assert.Equal(t, portalURL, *response.ConsentPortalURL)
	assert.NotNil(t, response.Fields)
	assert.Equal(t, 1, len(*response.Fields))
}

func TestConsentRecord_ToConsentResponseInternalView_Approved(t *testing.T) {
	record := ConsentRecord{
		ConsentID: uuid.New(),
		Status:    string(StatusApproved),
		Fields:    []ConsentField{{FieldName: "email", SchemaID: "schema-1", Owner: OwnerCitizen}},
	}

	response := record.ToConsentResponseInternalView()

	assert.Equal(t, record.ConsentID.String(), response.ConsentID)
	assert.Equal(t, string(StatusApproved), response.Status)
	assert.Nil(t, response.ConsentPortalURL) // Not pending, so no portal URL
	assert.NotNil(t, response.Fields)        // Approved status includes fields
}

func TestConsentRecord_ToConsentResponseInternalView_Rejected(t *testing.T) {
	record := ConsentRecord{
		ConsentID: uuid.New(),
		Status:    string(StatusRejected),
		Fields:    []ConsentField{{FieldName: "email", SchemaID: "schema-1", Owner: OwnerCitizen}},
	}

	response := record.ToConsentResponseInternalView()

	assert.Equal(t, record.ConsentID.String(), response.ConsentID)
	assert.Equal(t, string(StatusRejected), response.Status)
	assert.Nil(t, response.ConsentPortalURL)
	assert.Nil(t, response.Fields) // Rejected status doesn't include fields
}

func TestConsentRecord_ToConsentResponseInternalView_PendingNoURL(t *testing.T) {
	record := ConsentRecord{
		ConsentID:        uuid.New(),
		Status:           string(StatusPending),
		ConsentPortalURL: "", // Empty URL
		Fields:           []ConsentField{{FieldName: "email", SchemaID: "schema-1", Owner: OwnerCitizen}},
	}

	response := record.ToConsentResponseInternalView()

	assert.Nil(t, response.ConsentPortalURL) // Empty URL should not be included
}

func TestConsentRecord_ToConsentResponsePortalView(t *testing.T) {
	appName := "Test App"
	record := ConsentRecord{
		ConsentID:  uuid.New(),
		AppID:      "app-123",
		AppName:    &appName,
		OwnerID:    "owner-123",
		OwnerEmail: "owner@example.com",
		Status:     string(StatusApproved),
		Type:       string(TypeRealtime),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Fields:     []ConsentField{{FieldName: "email", SchemaID: "schema-1", Owner: OwnerCitizen}},
	}

	response := record.ToConsentResponsePortalView()

	assert.Equal(t, record.AppID, response.AppID)
	assert.NotNil(t, response.AppName)
	assert.Equal(t, appName, *response.AppName)
	assert.Equal(t, record.OwnerID, response.OwnerID)
	assert.Equal(t, record.OwnerEmail, response.OwnerEmail)
	assert.Equal(t, ConsentStatus(record.Status), response.Status)
	assert.Equal(t, ConsentType(record.Type), response.Type)
	assert.Equal(t, record.CreatedAt, response.CreatedAt)
	assert.Equal(t, record.UpdatedAt, response.UpdatedAt)
	assert.Equal(t, 1, len(response.Fields))
}

func TestConsentRecord_ToConsentResponsePortalView_NoAppName(t *testing.T) {
	record := ConsentRecord{
		ConsentID:  uuid.New(),
		AppID:      "app-123",
		AppName:    nil,
		OwnerID:    "owner-123",
		OwnerEmail: "owner@example.com",
		Status:     string(StatusApproved),
		Type:       string(TypeRealtime),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Fields:     []ConsentField{},
	}

	response := record.ToConsentResponsePortalView()

	assert.Nil(t, response.AppName)
}

