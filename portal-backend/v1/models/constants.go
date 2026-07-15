package models

// UserGroup represents different user groups in the system
type UserGroup string

const (
	UserGroupAdmin  UserGroup = "OpenDIF_Admin"
	UserGroupMember UserGroup = "OpenDIF_Members"
)

// Status represents the status of submissions and applications
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// Version represents application versioning states
type Version string

const (
	ActiveVersion     Version = "active"
	DeprecatedVersion Version = "deprecated"
)

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
)

// Field length constraints remain as regular constants
const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
	MaxEmailLength       = 320 // RFC 3696 specification
	MaxPhoneLength       = 15  // E.164 format
	MaxEndpointLength    = 2048
)

// IDP Application Constants
const (
	TemplateIDM2M = "m2m-application"
)
