package middleware

// AuditStatus represents the status of audit events
type AuditStatus string

const (
	AuditStatusSuccess AuditStatus = "SUCCESS"
	AuditStatusFailure AuditStatus = "FAILURE"
)

// ActorType represents different actor types for auditing
type ActorType string

const (
	ActorTypeAdmin  ActorType = "ADMIN"
	ActorTypeMember ActorType = "MEMBER"
	ActorTypeSystem ActorType = "SYSTEM"
)

// TargetType represents different target types for auditing
type TargetType string

const (
	TargetTypeService  TargetType = "SERVICE"
	TargetTypeResource TargetType = "RESOURCE"
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
