package caller

// OAuth2 scopes the Portal Backend checks. Each route in cmd/pb/main.go
// requires one of them; the PB README maps them to ThunderID roles.
//
// A scope is <resource>:<action>, matching how ThunderID builds a permission
// from a resource handle and an action handle on PB's resource server. They
// carry no namespace prefix: PB only accepts tokens whose audience is its own
// resource server, and ThunderID binds each token to exactly one resource
// server, so a scope in a PB token can only mean PB's.
//
// read and write cover the caller's own organization unit; read-all and
// write-all cover every organization unit.
const (
	ScopeMemberRead     = "member:read"
	ScopeMemberWrite    = "member:write"
	ScopeMemberReadAll  = "member:read-all"
	ScopeMemberWriteAll = "member:write-all"

	ScopeSchemaRead     = "schema:read"
	ScopeSchemaWrite    = "schema:write"
	ScopeSchemaReadAll  = "schema:read-all"
	ScopeSchemaWriteAll = "schema:write-all"

	ScopeSchemaSubmissionRead     = "schema-submission:read"
	ScopeSchemaSubmissionWrite    = "schema-submission:write"
	ScopeSchemaSubmissionReadAll  = "schema-submission:read-all"
	ScopeSchemaSubmissionWriteAll = "schema-submission:write-all"

	ScopeApplicationRead     = "application:read"
	ScopeApplicationWrite    = "application:write"
	ScopeApplicationReadAll  = "application:read-all"
	ScopeApplicationWriteAll = "application:write-all"

	ScopeApplicationSubmissionRead     = "application-submission:read"
	ScopeApplicationSubmissionWrite    = "application-submission:write"
	ScopeApplicationSubmissionReadAll  = "application-submission:read-all"
	ScopeApplicationSubmissionWriteAll = "application-submission:write-all"

	// Consent bypass grants let an application skip the owner's consent. They
	// span every organization unit: read lists them, and write grants and
	// revokes them.
	ScopeConsentBypassRead  = "consent-bypass:read"
	ScopeConsentBypassWrite = "consent-bypass:write"
)

// AllScopes is every PB scope. A client can request all of them: ThunderID
// grants only the ones the user's roles hold.
var AllScopes = []string{
	ScopeMemberRead, ScopeMemberWrite, ScopeMemberReadAll, ScopeMemberWriteAll,
	ScopeSchemaRead, ScopeSchemaWrite, ScopeSchemaReadAll, ScopeSchemaWriteAll,
	ScopeSchemaSubmissionRead, ScopeSchemaSubmissionWrite, ScopeSchemaSubmissionReadAll, ScopeSchemaSubmissionWriteAll,
	ScopeApplicationRead, ScopeApplicationWrite, ScopeApplicationReadAll, ScopeApplicationWriteAll,
	ScopeApplicationSubmissionRead, ScopeApplicationSubmissionWrite, ScopeApplicationSubmissionReadAll, ScopeApplicationSubmissionWriteAll,
	ScopeConsentBypassRead, ScopeConsentBypassWrite,
}
