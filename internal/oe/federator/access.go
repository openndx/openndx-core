package federator

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"

	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator/operation"
	"github.com/openndx/openndx-core/internal/oe/internals/errors"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
)

// authorizeAccess decides whether appID may read fields of op. It asks the PDP,
// and asks the data owner's consent when the PDP requires it. Only then is the
// Data Owner ID read from op, so a query that needs no consent may be keyed by
// any argument. It returns a non-nil response when the query must be rejected.
func (f *Federator) authorizeAccess(ctx context.Context, appID string, op *operation.Operation, fields []policy.RequiredField) (context.Context, *graphql.Response) {
	ctx, pdpResponse, denied := f.checkPolicy(ctx, appID, fields)
	if denied != nil {
		return ctx, denied
	}

	if pdpResponse == nil || !pdpResponse.AppRequiresOwnerConsent {
		return ctx, nil
	}

	// Consent that isn't set up is the deployment's fault, not the query's, so
	// it is reported before the Data Owner ID is looked for.
	if f.deps.Consent == nil {
		slog.WarnContext(ctx, "Consent required but no consent engine is configured")
		return ctx, rejectWithCode("Consent required but consent engine not available", errors.CodeCEError)
	}

	dataOwnerID, denied := f.dataOwnerID(ctx, op)
	if denied != nil {
		return ctx, denied
	}
	return f.checkConsent(ctx, appID, dataOwnerID, pdpResponse)
}

// reject returns the error response for a rejected query.
func reject(message string, extensions map[string]interface{}) *graphql.Response {
	resp := createErrorResponse(message, extensions)
	return &resp
}

// rejectWithCode returns the error response for a query rejected with an error code.
func rejectWithCode(message, code string) *graphql.Response {
	resp := createErrorResponseWithCode(message, code)
	return &resp
}

// checkPolicy asks the PDP whether appID may read fields. Without a PDP the check is
// skipped and the returned PDP response is nil.
func (f *Federator) checkPolicy(ctx context.Context, appID string, fields []policy.RequiredField) (context.Context, *policy.PdpResponse, *graphql.Response) {
	if f.deps.Policy == nil {
		slog.WarnContext(ctx, "PDP client not available, skipping policy check")
		// Continue without PDP check - this allows the system to work without PDP
		return ctx, nil, nil
	}

	pdpRequest := &policy.PdpRequest{
		AppId:          appID,
		RequiredFields: fields,
	}

	pdpResponse, err := f.deps.Policy.MakePdpRequest(ctx, pdpRequest)
	ctx = f.deps.Auditor.PolicyCheck(ctx, appID, pdpRequest, pdpResponse, err)

	if err != nil {
		slog.ErrorContext(ctx, "PDP request failed", "error", err)
		return ctx, nil, rejectWithCode(fmt.Sprintf("Authorization check failed: %v", err), errors.CodePDPError)
	}

	if pdpResponse == nil {
		slog.ErrorContext(ctx, "Failed to get response from PDP")
		return ctx, nil, rejectWithCode("No response from authorization service", errors.CodePDPNoResponse)
	}

	// Log PDP decision for audit trail
	slog.InfoContext(ctx, "PDP decision received",
		"authorized", pdpResponse.AppAuthorized,
		"consentRequired", pdpResponse.AppRequiresOwnerConsent,
		"unauthorizedFieldsCount", len(pdpResponse.UnauthorizedFields),
		"expiredFieldsCount", len(pdpResponse.ExpiredFields))

	if !pdpResponse.AppAuthorized {
		slog.InfoContext(ctx, "Request not authorized by PDP",
			"unauthorizedFields", pdpResponse.UnauthorizedFields)
		return ctx, nil, reject("Access denied", map[string]interface{}{
			"code":               errors.CodePDPNotAllowed,
			"unauthorizedFields": pdpResponse.UnauthorizedFields,
		})
	}

	if pdpResponse.AppAccessExpired {
		slog.InfoContext(ctx, "Application access expired",
			"expiredFields", pdpResponse.ExpiredFields)
		return ctx, nil, reject("Access expired", map[string]interface{}{
			"code":          errors.CodePDPNotAllowed,
			"expiredFields": pdpResponse.ExpiredFields,
		})
	}

	return ctx, pdpResponse, nil
}

// dataOwnerID returns the Data Owner ID: the value of the configured owner ID
// argument (e.g. "nic") on every field of op, aliases included.
func (f *Federator) dataOwnerID(ctx context.Context, op *operation.Operation) (string, *graphql.Response) {
	id, err := op.DataOwnerID(f.opts.OwnerIDArgName)
	switch {
	case stderrors.Is(err, operation.ErrMissingIdentifier):
		slog.InfoContext(ctx, "Data Owner ID argument is missing or invalid",
			"ownerIdArgName", f.opts.OwnerIDArgName)
		return "", rejectWithCode("Data Owner ID argument is missing or invalid", errors.CodeMissingEntityIdentifier)
	case stderrors.Is(err, operation.ErrAmbiguousIdentifier):
		slog.InfoContext(ctx, "Conflicting Data Owner ID values found in query",
			"ownerIdArgName", f.opts.OwnerIDArgName)
		return "", rejectWithCode("Conflicting Data Owner ID values in query", errors.CodeAmbiguousEntityIdentifier)
	}
	return id, nil
}

// checkConsent asks the Consent Engine for dataOwnerID's consent to the fields the
// PDP marked as requiring it, and rejects the query unless consent is approved.
func (f *Federator) checkConsent(ctx context.Context, appID, dataOwnerID string, pdpResponse *policy.PdpResponse) (context.Context, *graphql.Response) {
	slog.InfoContext(ctx, "Consent required for fields",
		"fieldsCount", len(pdpResponse.ConsentRequiredFields),
		"fields", pdpResponse.ConsentRequiredFields)

	if len(pdpResponse.ConsentRequiredFields) == 0 {
		slog.ErrorContext(ctx, "PDP indicates consent required but no fields specified")
		return ctx, rejectWithCode("Invalid PDP response: consent required but no fields specified", errors.CodePDPError)
	}

	// Map PDP response fields to Consent Engine request with all metadata
	fields := make([]consent.ConsentField, len(pdpResponse.ConsentRequiredFields))
	for i, field := range pdpResponse.ConsentRequiredFields {
		fields[i].FieldName = field.FieldName
		fields[i].SchemaID = field.SchemaID
		fields[i].DisplayName = field.DisplayName
		fields[i].Description = field.Description

		// Map Owner from PDP response, default to citizen if not provided
		if field.Owner != nil {
			fields[i].Owner = consent.OwnerType(*field.Owner)
		} else {
			fields[i].Owner = consent.OwnerCitizen
		}
	}

	typeRealTime := consent.TypeRealtime
	ceRequest := &consent.CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: consent.ConsentRequirement{
			Owner:   consent.OwnerCitizen,
			OwnerID: dataOwnerID,
			Fields:  fields,
		},
		ConsentType: &typeRealTime,
	}

	ceResp, err := f.deps.Consent.CreateConsent(ctx, ceRequest)
	ctx = f.deps.Auditor.ConsentCheck(ctx, appID, dataOwnerID, ceRequest, ceResp, err)

	if err != nil {
		slog.InfoContext(ctx, "CE request failed", "error", err)
		return ctx, rejectWithCode("CE request failed", errors.CodeCEError)
	}
	if ceResp == nil {
		slog.ErrorContext(ctx, "Failed to get response from CE")
		return ctx, rejectWithCode("Failed to get response from CE", errors.CodeCENoResponse)
	}

	slog.InfoContext(ctx, "Consent Response", "response", ceResp)

	if ceResp.Status != consent.StatusApproved {
		// Status is pending or any other non-approved status
		slog.InfoContext(ctx, "Consent not approved", "status", ceResp.Status)
		return ctx, reject("Consent not approved", map[string]interface{}{
			"code":             errors.CodeCENotApproved,
			"consentPortalUrl": ceResp.ConsentPortalURL,
			"consentStatus":    ceResp.Status,
		})
	}

	slog.InfoContext(ctx, "Consent approved, proceeding with query execution")
	return ctx, nil
}
