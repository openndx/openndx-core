package decision

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFields is a FieldReader over a fixed set of field policies.
type fakeFields struct {
	policies []FieldPolicy
	err      error
	asked    []FieldKey
}

func (f *fakeFields) FieldPolicies(_ context.Context, keys []FieldKey) ([]FieldPolicy, error) {
	f.asked = keys
	if f.err != nil {
		return nil, f.err
	}
	wanted := make(map[FieldKey]bool, len(keys))
	for _, k := range keys {
		wanted[k] = true
	}
	var out []FieldPolicy
	for _, p := range f.policies {
		if wanted[p.FieldKey] {
			out = append(out, p)
		}
	}
	return out, nil
}

// fakeGrants is a GrantReader over a fixed set of grants per application.
type fakeGrants struct {
	byApp map[string][]FieldGrant
	err   error
	asked []FieldKey
}

func (f *fakeGrants) ActiveGrants(_ context.Context, applicationID string, keys []FieldKey) ([]FieldGrant, error) {
	f.asked = keys
	if f.err != nil {
		return nil, f.err
	}
	return f.byApp[applicationID], nil
}

var testNow = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func newFakeService(fields *fakeFields, grants *fakeGrants) *Service {
	s := NewService(fields, grants)
	s.now = func() time.Time { return testNow }
	return s
}

func key(field string) FieldKey { return FieldKey{SchemaID: "s1", FieldName: field} }

func policy(field string, allowlist, consent bool) FieldPolicy {
	return FieldPolicy{FieldKey: key(field), Policy: Field{RequiresAllowlist: allowlist, RequiresConsent: consent}}
}

func request(app string, fields ...string) *PolicyDecisionRequest {
	req := &PolicyDecisionRequest{ApplicationID: app}
	for _, f := range fields {
		req.RequiredFields = append(req.RequiredFields, PolicyDecisionRequestRecord{SchemaID: "s1", FieldName: f})
	}
	return req
}

func TestService_GetPolicyDecision_OutcomeMapping(t *testing.T) {
	owner := "citizen"
	consentField := policy("consent", false, true)
	consentField.DisplayName, consentField.Description, consentField.Owner = new("Consent"), new("Needs consent"), &owner
	fields := &fakeFields{policies: []FieldPolicy{
		policy("open", false, false),
		policy("allowed", true, false),
		policy("expired", true, false),
		policy("denied", true, false),
		consentField,
		policy("bypassed", true, true),
	}}
	grants := &fakeGrants{byApp: map[string][]FieldGrant{"app": {
		{FieldKey: key("allowed"), Grant: Grant{ExpiresAt: testNow.Add(time.Hour)}},
		{FieldKey: key("expired"), Grant: Grant{ExpiresAt: testNow.Add(-time.Hour)}},
		{FieldKey: key("bypassed"), Grant: Grant{ExpiresAt: testNow.Add(time.Hour), ConsentBypass: true}},
	}}}

	order := []string{"denied", "open", "allowed", "expired", "consent", "bypassed"}
	resp, err := newFakeService(fields, grants).GetPolicyDecision(t.Context(), request("app", order...))
	require.NoError(t, err)

	wantKeys := make([]FieldKey, 0, len(order))
	for _, f := range order {
		wantKeys = append(wantKeys, key(f))
	}
	assert.Equal(t, wantKeys, fields.asked, "fields are read for the requested keys")
	assert.Equal(t, wantKeys, grants.asked, "grants are read for the requested keys")

	outcome := func(field string, o Outcome, reason string, obligations ...string) PolicyDecisionFieldOutcome {
		return PolicyDecisionFieldOutcome{SchemaID: "s1", FieldName: field, Outcome: string(o), Reason: reason, Obligations: obligations}
	}
	assert.Equal(t, []PolicyDecisionFieldOutcome{
		outcome("denied", OutcomeDeny, ReasonNotAllowlisted),
		outcome("open", OutcomeAllow, ""),
		outcome("allowed", OutcomeAllow, ""),
		outcome("expired", OutcomeExpired, ReasonGrantExpired),
		outcome("consent", OutcomeConsentRequired, ""),
		outcome("bypassed", OutcomeAllow, ReasonConsentBypassed, ObligationAuditConsentBypassed),
	}, resp.Fields, "one outcome per requested field, in request order")

	assert.Equal(t, []PolicyDecisionResponseFieldRecord{{SchemaID: "s1", FieldName: "denied"}}, resp.UnauthorizedFields)
	assert.Equal(t, []PolicyDecisionResponseFieldRecord{{SchemaID: "s1", FieldName: "expired"}}, resp.ExpiredFields)
	assert.Equal(t, []PolicyDecisionResponseFieldRecord{{
		SchemaID: "s1", FieldName: "consent", DisplayName: new("Consent"), Description: new("Needs consent"), Owner: &owner,
	}}, resp.ConsentRequiredFields)
	assert.False(t, resp.AppAuthorized)
	assert.True(t, resp.AppAccessExpired)
	assert.True(t, resp.AppRequiresOwnerConsent)
}

func TestService_GetPolicyDecision_AllAllowed(t *testing.T) {
	fields := &fakeFields{policies: []FieldPolicy{policy("open", false, false)}}
	resp, err := newFakeService(fields, &fakeGrants{}).GetPolicyDecision(t.Context(), request("app", "open"))
	require.NoError(t, err)

	assert.True(t, resp.AppAuthorized)
	assert.False(t, resp.AppAccessExpired)
	assert.False(t, resp.AppRequiresOwnerConsent)
	assert.Nil(t, resp.UnauthorizedFields)
	assert.Nil(t, resp.ExpiredFields)
	assert.Nil(t, resp.ConsentRequiredFields)
}

func TestService_GetPolicyDecision_NoApplicationDenies(t *testing.T) {
	fields := &fakeFields{policies: []FieldPolicy{policy("open", false, false)}}
	resp, err := newFakeService(fields, &fakeGrants{}).GetPolicyDecision(t.Context(), request("", "open"))
	require.NoError(t, err)

	assert.Equal(t, string(OutcomeDeny), resp.Fields[0].Outcome)
	assert.Equal(t, ReasonNoApplication, resp.Fields[0].Reason)
	assert.False(t, resp.AppAuthorized)
}

func TestService_GetPolicyDecision_FieldNotFound(t *testing.T) {
	fields := &fakeFields{policies: []FieldPolicy{policy("open", false, false)}}
	_, err := newFakeService(fields, &fakeGrants{}).GetPolicyDecision(t.Context(), request("app", "open", "missing"))

	require.ErrorIs(t, err, ErrFieldNotFound)
	assert.EqualError(t, err, "policy metadata not found for schema_id s1 and field_name missing")
}

func TestService_GetPolicyDecision_ReaderErrors(t *testing.T) {
	boom := errors.New("boom")

	_, err := newFakeService(&fakeFields{err: boom}, &fakeGrants{}).GetPolicyDecision(t.Context(), request("app", "f"))
	assert.ErrorIs(t, err, boom)

	fields := &fakeFields{policies: []FieldPolicy{policy("f", false, false)}}
	_, err = newFakeService(fields, &fakeGrants{err: boom}).GetPolicyDecision(t.Context(), request("app", "f"))
	assert.ErrorIs(t, err, boom)
}

func TestService_GetPolicyDecision_PrefersUnexpiredBypassGrant(t *testing.T) {
	fields := &fakeFields{policies: []FieldPolicy{policy("f", true, true)}}
	plain := FieldGrant{FieldKey: key("f"), Grant: Grant{ExpiresAt: testNow.Add(time.Hour)}}
	bypass := FieldGrant{FieldKey: key("f"), Grant: Grant{ExpiresAt: testNow.Add(time.Hour), ConsentBypass: true}}
	expiredBypass := FieldGrant{FieldKey: key("f"), Grant: Grant{ExpiresAt: testNow.Add(-time.Hour), ConsentBypass: true}}

	decide := func(grants ...FieldGrant) PolicyDecisionFieldOutcome {
		t.Helper()
		svc := newFakeService(fields, &fakeGrants{byApp: map[string][]FieldGrant{"app": grants}})
		resp, err := svc.GetPolicyDecision(t.Context(), request("app", "f"))
		require.NoError(t, err)
		return resp.Fields[0]
	}

	assert.Equal(t, ReasonConsentBypassed, decide(plain, bypass).Reason)
	assert.Equal(t, ReasonConsentBypassed, decide(bypass, plain).Reason)
	assert.Equal(t, string(OutcomeConsentRequired), decide(expiredBypass, plain).Outcome)
	assert.Equal(t, string(OutcomeConsentRequired), decide(plain, expiredBypass).Outcome)
	assert.Equal(t, string(OutcomeExpired), decide(expiredBypass).Outcome)
}

func TestPreferredGrant(t *testing.T) {
	now := time.Now()
	plain := &Grant{ExpiresAt: now.Add(time.Hour)}
	bypass := &Grant{ConsentBypass: true, ExpiresAt: now.Add(time.Hour)}
	expiredBypass := &Grant{ConsentBypass: true, ExpiresAt: now.Add(-time.Hour)}

	assert.Same(t, plain, preferredGrant(nil, plain, now))
	assert.Same(t, bypass, preferredGrant(plain, bypass, now))
	assert.Same(t, bypass, preferredGrant(bypass, plain, now))
	assert.Same(t, plain, preferredGrant(expiredBypass, plain, now))
	assert.Same(t, plain, preferredGrant(plain, expiredBypass, now))
	assert.Same(t, expiredBypass, preferredGrant(nil, expiredBypass, now))
}
