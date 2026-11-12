package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAccessGrant_ToResponse(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+1800)
	g := AccessGrant{
		ID:            uuid.New(),
		SchemaID:      "schema-123",
		FieldName:     "person.fullName",
		ApplicationID: "client-1",
		ExpiresAt:     time.Date(2026, 2, 1, 5, 30, 0, 0, ist),
		Status:        GrantStatusActive,
		CreatedAt:     time.Date(2026, 1, 1, 5, 30, 0, 0, ist),
		UpdatedAt:     time.Date(2026, 1, 2, 5, 30, 0, 0, ist),
	}

	assert.Equal(t, "access_grants", g.TableName())
	assert.Equal(t, AccessGrantResponse{
		ID:            g.ID.String(),
		SchemaID:      "schema-123",
		FieldName:     "person.fullName",
		ApplicationID: "client-1",
		ExpiresAt:     "2026-02-01T00:00:00Z",
		Status:        GrantStatusActive,
		CreatedAt:     "2026-01-01T00:00:00Z",
		UpdatedAt:     "2026-01-02T00:00:00Z",
	}, g.ToResponse())
}

func TestAccessGrant_ToResponse_ConsentBypass(t *testing.T) {
	legalBasis := "Police Act s.12"
	g := AccessGrant{
		ID:            uuid.New(),
		Status:        GrantStatusActive,
		ConsentBypass: true,
		LegalBasis:    &legalBasis,
	}

	response := g.ToResponse()
	assert.True(t, response.ConsentBypass)
	assert.Equal(t, &legalBasis, response.LegalBasis)
}
