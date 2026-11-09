package middleware

import (
	"context"

	auditpkg "github.com/LSFLK/argus/pkg/audit"
	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/monitoring"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
)

// Auditor sends the orchestration engine's audit events to an argus audit client.
// It implements federator.Auditor.
type Auditor struct {
	client    auditpkg.Auditor
	actorType string
	actorID   string
}

var _ federator.Auditor = (*Auditor)(nil)

// NewAuditor returns an Auditor that logs through client and attributes service-initiated
// events to actorType and actorID (from config.AuditConfig). A nil client records nothing.
// Note: targetType is not stored here as it varies per API call
func NewAuditor(client auditpkg.Auditor, actorType, actorID string) *Auditor {
	return &Auditor{
		client:    client,
		actorType: actorType,
		actorID:   actorID,
	}
}

// logEvent hands the event to the audit client, which sends it asynchronously.
func (a *Auditor) logEvent(ctx context.Context, auditRequest *auditpkg.AuditLogRequest) {
	if a.client == nil {
		return
	}
	a.client.LogEvent(ctx, auditRequest)
}

// maxAuditErrorsToLog is the maximum number of errors to include in audit log metadata
// This limit prevents audit logs from becoming too large when there are many errors
const maxAuditErrorsToLog = 3

// withTraceID returns ctx and its trace ID, generating one when ctx has none.
func withTraceID(ctx context.Context) (context.Context, string) {
	traceID := monitoring.GetTraceIDFromContext(ctx)
	if traceID == "" {
		traceID = uuid.New().String()
		ctx = monitoring.WithTraceID(ctx, traceID)
	}
	return ctx, traceID
}

// RequestReceived logs a DATA_REQUEST event for a consumer query entering the orchestration engine.
// Returns the updated context with traceID to ensure trace correlation
func (a *Auditor) RequestReceived(ctx context.Context, appID, query string) context.Context {
	ctx, traceID := withTraceID(ctx)

	// The consumer application is the actor; there is no target for the entry point
	targetID := "SERVICE"
	auditRequest := &auditpkg.AuditLogRequest{
		TraceID:    &traceID,
		Timestamp:  auditpkg.CurrentTimestamp(),
		EventType:  "DATA_REQUEST",
		Status:     auditpkg.StatusSuccess,
		ActorType:  "APPLICATION",
		ActorID:    appID,
		TargetType: "SERVICE",
		TargetID:   &targetID,
		Metadata: map[string]interface{}{
			"requestMetadata": map[string]interface{}{
				"applicationId": appID,
				"query":         query,
			},
		},
	}

	a.logEvent(ctx, auditRequest)
	return ctx
}

// PolicyCheck logs a POLICY_CHECK event after a request to the Policy Decision Point.
// Returns the updated context with traceID to ensure trace correlation
func (a *Auditor) PolicyCheck(ctx context.Context, appID string, req *policy.PdpRequest, resp *policy.PdpResponse, err error) context.Context {
	status := auditpkg.StatusSuccess
	requestMetadata := map[string]interface{}{"applicationId": appID}
	responseMetadata := make(map[string]interface{})

	if req != nil {
		requestMetadata["requiredFields"] = req.RequiredFields
	}

	if err != nil {
		status = auditpkg.StatusFailure
		responseMetadata["error"] = err.Error()
	} else if resp != nil {
		responseMetadata["authorized"] = resp.AppAuthorized
		responseMetadata["consentRequired"] = resp.AppRequiresOwnerConsent
		responseMetadata["accessExpired"] = resp.AppAccessExpired
		// Per-field outcomes, with any obligations the policy returned.
		// Absent when the PDP predates per-field outcomes.
		if resp.Fields != nil {
			responseMetadata["fields"] = resp.Fields
		}
		if !resp.AppAuthorized {
			status = auditpkg.StatusFailure
			responseMetadata["unauthorizedFields"] = resp.UnauthorizedFields
		}
		if resp.AppAccessExpired {
			status = auditpkg.StatusFailure
			responseMetadata["expiredFields"] = resp.ExpiredFields
		}
		if resp.AppRequiresOwnerConsent {
			responseMetadata["consentRequiredFields"] = resp.ConsentRequiredFields
		}
	}

	// Policy Decision Point is a service, so targetType is "SERVICE"
	return a.logServiceEvent(ctx, "POLICY_CHECK", "policy-decision-point", requestMetadata, responseMetadata, status)
}

// ConsentCheck logs a CONSENT_CHECK event after a request to the Consent Engine.
// Returns the updated context with traceID to ensure trace correlation
func (a *Auditor) ConsentCheck(ctx context.Context, appID, ownerID string, req *consent.CreateConsentRequest, resp *consent.ConsentResponseInternalView, err error) context.Context {
	status := auditpkg.StatusSuccess
	requestMetadata := map[string]interface{}{"applicationId": appID}
	responseMetadata := make(map[string]interface{})

	if ownerID != "" {
		requestMetadata["ownerId"] = ownerID
	}
	if req != nil {
		requestMetadata["fieldsCount"] = len(req.ConsentRequirement.Fields)
	}

	if err != nil {
		status = auditpkg.StatusFailure
		responseMetadata["error"] = err.Error()
	} else if resp != nil {
		responseMetadata["consentId"] = resp.ConsentID
		responseMetadata["status"] = resp.Status
		if resp.ConsentPortalURL != nil {
			responseMetadata["consentPortalUrl"] = *resp.ConsentPortalURL
		}
	}

	// Consent Engine is a service, so targetType is "SERVICE"
	return a.logServiceEvent(ctx, "CONSENT_CHECK", "consent-engine", requestMetadata, responseMetadata, status)
}

// ProviderFetch logs a PROVIDER_FETCH event after a sub-query to a provider.
func (a *Auditor) ProviderFetch(ctx context.Context, fetch federator.ProviderFetch, response *graphql.Response, err error) {
	requestedFields := fetch.RequestedFields
	if requestedFields == nil {
		requestedFields = []string{}
	}

	// Combine requested data and additional info into response metadata
	// (since we're logging after receiving the response)
	responseMetadata := map[string]interface{}{
		"applicationId":   fetch.ApplicationID,
		"schemaId":        fetch.SchemaID,
		"requestedFields": requestedFields,
		"query":           fetch.Query,
	}
	if err != nil {
		responseMetadata["error"] = err.Error()
	}
	if response != nil {
		responseMetadata["hasErrors"] = len(response.Errors) > 0
		if len(response.Errors) > 0 {
			responseMetadata["errorCount"] = len(response.Errors)
			// Include first few errors (limit to avoid large payloads)
			errorDetails := make([]interface{}, 0)
			for i, gqlErr := range response.Errors {
				if i >= maxAuditErrorsToLog {
					break
				}
				errorDetails = append(errorDetails, gqlErr)
			}
			responseMetadata["errors"] = errorDetails
		}
		if response.Data != nil {
			// Include data keys for reference (not full data to avoid large payloads)
			dataKeys := make([]string, 0, len(response.Data))
			for key := range response.Data {
				dataKeys = append(dataKeys, key)
			}
			responseMetadata["dataKeys"] = dataKeys
		}
	}

	auditStatus := auditpkg.StatusSuccess
	if err != nil || (response != nil && len(response.Errors) > 0) {
		auditStatus = auditpkg.StatusFailure
	}

	// Providers are services, so targetType is "SERVICE"
	a.logServiceEvent(ctx, "PROVIDER_FETCH", fetch.SchemaID, nil, responseMetadata, auditStatus)
}

// logServiceEvent logs an event in which the orchestration engine (the configured actor)
// calls the service targetID. Returns the updated context with traceID (if one was generated)
// to ensure trace correlation.
func (a *Auditor) logServiceEvent(ctx context.Context, eventType, targetID string, requestMetadata, responseMetadata map[string]interface{}, status string) context.Context {
	ctx, traceID := withTraceID(ctx)

	meta := make(map[string]interface{})
	if len(requestMetadata) > 0 {
		meta["requestMetadata"] = requestMetadata
	}
	if len(responseMetadata) > 0 {
		meta["responseMetadata"] = responseMetadata
	}

	auditRequest := &auditpkg.AuditLogRequest{
		TraceID:    &traceID,
		Timestamp:  auditpkg.CurrentTimestamp(),
		EventType:  eventType,
		Status:     status,
		ActorType:  a.actorType,
		ActorID:    a.actorID,
		TargetType: "SERVICE",
		TargetID:   &targetID,
		Metadata:   meta,
	}

	a.logEvent(ctx, auditRequest)

	// Return the updated context to ensure traceID correlation across the request flow
	return ctx
}
