// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package errors

// PDP-related
const (
	CodePDPNotAllowed = "PDP_NOT_ALLOWED"
	CodePDPError      = "PDP_ERROR"
	CodePDPNoResponse = "PDP_NO_RESPONSE"
)

// CE-related
const (
	CodeCEError       = "CE_ERROR"
	CodeCENoResponse  = "CE_NO_RESPONSE"
	CodeCENotApproved = "CE_NOT_APPROVED"
)

// OE-related
const (
	CodeMissingEntityIdentifier   = "MISSING_IDENTIFIER"
	CodeAmbiguousEntityIdentifier = "AMBIGUOUS_IDENTIFIER"
	// CodeProviderError marks fields a provider failed to serve, and errors a provider returned.
	CodeProviderError = "PROVIDER_ERROR"
)

// Generic
const (
	CodeBadRequest = "BAD_REQUEST"
)
