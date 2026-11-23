// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package consent

// OwnerType represents the owner enum (matches PolicyDecisionPoint Owner type)
type OwnerType string

const (
	OwnerCitizen OwnerType = "citizen"
)

// ConsentStatus represents the status of a consent record
type ConsentStatus string

// ConsentStatus constants
const (
	StatusPending  ConsentStatus = "pending"
	StatusApproved ConsentStatus = "approved"
	StatusRejected ConsentStatus = "rejected"
	StatusExpired  ConsentStatus = "expired"
	StatusRevoked  ConsentStatus = "revoked"
)

// ConsentType represents the type of consent mechanism
type ConsentType string

// ConsentType constants
const (
	TypeRealtime ConsentType = "realtime"
	TypeOffline  ConsentType = "offline"
)

// Endpoint paths
const (
	consentEndpointPath = "/consents"
)
