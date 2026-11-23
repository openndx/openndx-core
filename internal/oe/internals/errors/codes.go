// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package errors

// PDP-related
const (
	CodePDPNotAllowed  = "PDP_NOT_ALLOWED"
	CodePDPUnavailable = "PDP_UNAVAILABLE"
	CodePDPError       = "PDP_ERROR"
	CodePDPNoResponse  = "PDP_NO_RESPONSE"
)

// CE-related
const (
	CodeCEUnavailable    = "CE_UNAVAILABLE"
	CodeCEError          = "CE_ERROR"
	CodeCENoResponse     = "CE_NO_RESPONSE"
	CodeCEConsentDenied  = "CE_CONSENT_DENIED"
	CodeCEConsentExpired = "CE_CONSENT_EXPIRED"
	CodeCENotApproved    = "CE_NOT_APPROVED"
)

// OE-related
const (
	CodeMissingEntityIdentifier   = "MISSING_IDENTIFIER"
	CodeAmbiguousEntityIdentifier = "AMBIGUOUS_IDENTIFIER"
	// CodeProviderError marks fields a provider failed to serve, and errors a provider returned.
	CodeProviderError = "PROVIDER_ERROR"
)

// Auth-related
const (
	CodeUnauthorized = "UNAUTHORIZED"
	CodeForbidden    = "FORBIDDEN"
)

// Generic
const (
	CodeInternalError = "INTERNAL_ERROR"
	CodeBadRequest    = "BAD_REQUEST"
)
