// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package middleware

import "github.com/openndx/openndx-core/internal/audit"

// AuditStatus represents the status of audit events
type AuditStatus string

const (
	AuditStatusSuccess = AuditStatus(audit.StatusSuccess)
	AuditStatusFailure = AuditStatus(audit.StatusFailure)
)

// ActorType represents different actor types for auditing
type ActorType string

const (
	ActorTypeAdmin  ActorType = "ADMIN"
	ActorTypeMember ActorType = "MEMBER"
	ActorTypeSystem ActorType = "SYSTEM"
)

// ResourceType represents different resource types for auditing
type ResourceType string

const (
	ResourceTypeMembers                ResourceType = "MEMBERS"
	ResourceTypeSchemas                ResourceType = "SCHEMAS"
	ResourceTypeSchemaSubmissions      ResourceType = "SCHEMA-SUBMISSIONS"
	ResourceTypeApplications           ResourceType = "APPLICATIONS"
	ResourceTypeApplicationSubmissions ResourceType = "APPLICATION-SUBMISSIONS"
	ResourceTypeConsentBypassGrants    ResourceType = "CONSENT-BYPASS-GRANTS"
)
