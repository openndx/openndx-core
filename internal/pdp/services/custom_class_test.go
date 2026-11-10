package services

import (
	"testing"

	"github.com/openndx/openndx-core/internal/pdp/engine"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCustomPolicyClass checks that a class loaded from a modules directory
// is accepted on write and decides its fields, using the example class.
func TestCustomPolicyClass(t *testing.T) {
	policyEngine, err := engine.New(t.Context(), engine.WithModulesDir("../../../cmd/pdp/examples/policies"))
	require.NoError(t, err)

	request := &models.PolicyMetadataCreateRequest{
		SchemaID: "s1",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName: "open_window", Source: models.SourcePrimary, PolicyClass: "access_window",
				PolicyParams: models.PolicyParams{"notBefore": "2000-01-01T00:00:00Z", "notAfter": "2100-01-01T00:00:00Z"},
			},
			{
				FieldName: "closed_window", Source: models.SourcePrimary, PolicyClass: "access_window",
				PolicyParams: models.PolicyParams{"notBefore": "2000-01-01T00:00:00Z", "notAfter": "2001-01-01T00:00:00Z"},
			},
		},
	}

	// A PDP without the modules directory rejects the class and names the
	// classes it has.
	builtInOnly := NewPolicyMetadataService(setupTestDB(t), testhelpers.Engine(t))
	_, err = builtInOnly.CreatePolicyMetadata(request)
	require.ErrorIs(t, err, ErrUnknownPolicyClass)
	assert.NotContains(t, err.Error(), "access_window,")

	service := NewPolicyMetadataService(setupTestDB(t), policyEngine)
	_, err = service.CreatePolicyMetadata(request)
	require.NoError(t, err)

	// An unknown class is still rejected, and the error lists the custom class.
	_, err = service.CreatePolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: "s2",
		Records:  []models.PolicyMetadataCreateRequestRecord{{FieldName: "f", Source: models.SourcePrimary, PolicyClass: "no_such_class"}},
	})
	require.ErrorIs(t, err, ErrUnknownPolicyClass)
	assert.Contains(t, err.Error(), "access_window, allowlist, allowlist_consent, consent, open")

	// The class denies an application without a grant.
	assert.Equal(t,
		models.PolicyDecisionFieldOutcome{SchemaID: "s1", FieldName: "open_window", Outcome: "deny", Reason: "not_allowlisted"},
		decideField(t, service, "app-1", "open_window"))

	// With a grant, the field's own parameters decide.
	for _, field := range []string{"open_window", "closed_window"} {
		grantPlain(t, service, "app-1", models.GrantDurationTypeOneMonth, field)
	}
	assert.Equal(t,
		models.PolicyDecisionFieldOutcome{SchemaID: "s1", FieldName: "open_window", Outcome: "allow"},
		decideField(t, service, "app-1", "open_window"))
	assert.Equal(t,
		models.PolicyDecisionFieldOutcome{SchemaID: "s1", FieldName: "closed_window", Outcome: "deny", Reason: "outside_access_window"},
		decideField(t, service, "app-1", "closed_window"))

	// A denied field makes the application unauthorized, as for built-in classes.
	resp, err := service.GetPolicyDecision(t.Context(), &models.PolicyDecisionRequest{
		ApplicationID: "app-1",
		RequiredFields: []models.PolicyDecisionRequestRecord{
			{SchemaID: "s1", FieldName: "open_window"},
			{SchemaID: "s1", FieldName: "closed_window"},
		},
	})
	require.NoError(t, err)
	assert.False(t, resp.AppAuthorized)
	require.Len(t, resp.UnauthorizedFields, 1)
	assert.Equal(t, "closed_window", resp.UnauthorizedFields[0].FieldName)
}
