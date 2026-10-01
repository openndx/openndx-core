package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/openndx/openndx-core/internal/pb/idp"
	"github.com/openndx/openndx-core/internal/pb/idp/idpfactory"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/pb/models"
	"github.com/openndx/openndx-core/internal/pb/services"
	"github.com/openndx/openndx-core/internal/pb/shared/utils"
	"gorm.io/gorm"
)

// V1Handler handles all V1 API routes
type V1Handler struct {
	memberService      *services.MemberService
	applicationService *services.ApplicationService
	schemaService      *services.SchemaService
}

// getUserMemberID gets the member ID for the authenticated user with caching
// This avoids repeated database calls for the same user within the same request context
func (h *V1Handler) getUserMemberID(r *http.Request, user *models.AuthenticatedUser) (string, error) {
	// Check if we already have cached the member ID
	if memberID, cached := user.GetCachedMemberID(); cached {
		// Return cached error if the previous lookup failed
		if err := user.GetCachedMemberIDError(); err != nil {
			return "", err
		}
		return memberID, nil
	}

	// Not cached, perform the database lookup
	members, err := h.memberService.GetAllMembers(r.Context(), &user.IdpUserID, nil)
	if err != nil {
		user.SetCachedMemberID("", err)
		return "", err
	}

	if len(members) == 0 {
		err = fmt.Errorf("user member record not found")
		user.SetCachedMemberID("", err)
		return "", err
	}

	// Cache the successful result
	memberID := members[0].MemberID
	user.SetCachedMemberID(memberID, nil)
	return memberID, nil
}

// NewV1Handler creates a new V1 handler
func NewV1Handler(db *gorm.DB) (*V1Handler, error) {
	// Get scopes from environment variable, fallback to default if not set
	scopesEnv := os.Getenv("IDP_SCOPE")
	var scopes []string
	if scopesEnv != "" {
		// Split by space to handle multiple scopes
		scopes = strings.Fields(scopesEnv)
	}
	// Create the NewIdpProvider
	baseURL := os.Getenv("IDP_BASE_URL")
	jwksURL := os.Getenv("IDP_JWKS_URL")
	issuerURL := os.Getenv("IDP_ISSUER")
	tokenURL := os.Getenv("IDP_TOKEN_URL")

	if baseURL == "" {
		if jwksURL != "" && (issuerURL != "" || tokenURL != "") {
			if issuerURL != "" {
				baseURL = issuerURL
			} else {
				baseURL = tokenURL
			}
		}
	}

	clientID := os.Getenv("IDP_CLIENT_ID")
	clientSecret := os.Getenv("IDP_CLIENT_SECRET")

	if baseURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("failed to create IDP provider: missing required environment variables (IDP_BASE_URL, or IDP_JWKS_URL and IDP_ISSUER/IDP_TOKEN_URL, along with IDP_CLIENT_ID and IDP_CLIENT_SECRET)")
	}

	idpProvider, err := idpfactory.NewIdpAPIProvider(idpfactory.FactoryConfig{
		ProviderType: idp.ProviderAsgardeo,
		BaseURL:      baseURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create IDP provider: %w", err)
	}
	memberService := services.NewMemberService(db, idpProvider)

	pdpServiceURL := os.Getenv("PDP_SERVICE_URL")
	if pdpServiceURL == "" {
		return nil, fmt.Errorf("PDP_SERVICE_URL environment variable not set")
	}
	if !strings.HasPrefix(pdpServiceURL, "http://") && !strings.HasPrefix(pdpServiceURL, "https://") {
		return nil, fmt.Errorf("PDP_SERVICE_URL must start with http:// or https://")
	}

	pdpService := services.NewPDPService(pdpServiceURL)
	slog.Info("PDP Service URL", "url", pdpServiceURL)

	return &V1Handler{
		memberService:      memberService,
		schemaService:      services.NewSchemaService(db, pdpService),
		applicationService: services.NewApplicationService(db, pdpService, idpProvider),
	}, nil
}

// Member handlers

// CreateMember handles POST /api/v1/members
func (h *V1Handler) CreateMember(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission - only admin users can create members
	if !user.HasPermission(models.PermissionCreateMember) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req models.CreateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Only admin users should reach this point due to permission check above
	// Admin users can create members for any user if IdpUserID is provided in the request

	member, err := h.memberService.CreateMember(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeMembers), nil, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeMembers), &member.MemberID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, member)
}

// UpdateMember handles PUT /api/v1/members/{memberId}
func (h *V1Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	memberId := r.PathValue("memberId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Get the existing member to check ownership
	existingMember, err := h.memberService.GetMember(r.Context(), memberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// Check if user can update this member resource
	// Admin can update any member, regular members can only update their own
	if !user.IsAdmin() && existingMember.IdpUserID != user.IdpUserID {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
		return
	}

	var req models.UpdateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Pass request context to service for proper context propagation
	member, err := h.memberService.UpdateMember(r.Context(), memberId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeMembers), &existingMember.MemberID, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeMembers), &member.MemberID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, member)
}

// GetMember handles GET /api/v1/members/{memberId}
func (h *V1Handler) GetMember(w http.ResponseWriter, r *http.Request) {
	memberId := r.PathValue("memberId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Get the member from database
	// Pass request context to service for proper context propagation
	member, err := h.memberService.GetMember(r.Context(), memberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// Check if user can access this member resource
	// Admin can access any member, regular members can only access their own
	if !user.IsAdmin() && member.IdpUserID != user.IdpUserID {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, member)
}

// GetAllMembers handles GET /api/v1/members
func (h *V1Handler) GetAllMembers(w http.ResponseWriter, r *http.Request) {
	idpUserId := r.URL.Query().Get("idpUserId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission - admin can read all members, regular users need specific permission
	var filteredIdpUserId *string

	if user.HasPermission(models.PermissionReadAllMembers) {
		// Admin can use provided filters or see all
		filteredIdpUserId = &idpUserId
		// Note: The email query parameter is accepted but not used,
		// since IdpUserID filtering is sufficient for uniqueness
	} else if user.HasPermission(models.PermissionReadMember) {
		// Regular users can only see their own member record
		// IdpUserID is unique, so no need to also filter by email
		filteredIdpUserId = &user.IdpUserID
	} else {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Pass request context to service for proper context propagation
	// Since IdpUserID is unique, we don't need to pass email parameter
	members, err := h.memberService.GetAllMembers(r.Context(), filteredIdpUserId, nil)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := models.CollectionResponse{
		Items: members,
		Count: len(members),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// Schema submission handlers

// GetAllSchemaSubmissions handles GET /api/v1/schema-submissions
func (h *V1Handler) GetAllSchemaSubmissions(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")
	statusFilter := r.URL.Query()["status"]

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	var filteredMemberId *string
	if user.HasPermission(models.PermissionReadAllSchemaSubmissions) {
		// Admin/System can use provided filters or see all
		filteredMemberId = &memberId
	} else if user.HasPermission(models.PermissionReadSchemaSubmission) {
		// Regular users can only see their own submissions
		// Get member ID for the authenticated user (cached)
		userMemberId, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		filteredMemberId = &userMemberId
	} else {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	submissions, err := h.schemaService.GetSchemaSubmissions(filteredMemberId, &statusFilter)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := models.CollectionResponse{
		Items: submissions,
		Count: len(submissions),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetSchemaSubmission handles GET /api/v1/schema-submissions/{submissionId}
func (h *V1Handler) GetSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionReadSchemaSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	submission, err := h.schemaService.GetSchemaSubmission(submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if submission belongs to the user
		if submission.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// CreateSchemaSubmission handles POST /api/v1/schema-submissions
func (h *V1Handler) CreateSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionCreateSchemaSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req models.CreateSchemaSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// For non-admin users, ensure they can only create submissions for themselves
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// If MemberID is provided, validate ownership
		if req.MemberID != "" {
			// Check if the provided MemberID belongs to the authenticated user
			if req.MemberID != userMemberID {
				utils.RespondWithError(w, http.StatusForbidden, "Access denied: cannot create submission for another user")
				return
			}
		} else {
			// If no MemberID provided, set it to the authenticated user's member ID
			req.MemberID = userMemberID
		}
	}

	submission, err := h.schemaService.CreateSchemaSubmission(&req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeSchemaSubmissions), nil, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeSchemaSubmissions), &submission.SubmissionID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, submission)
}

// UpdateSchemaSubmission handles PUT /api/v1/schema-submissions/{submissionId}
func (h *V1Handler) UpdateSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionUpdateSchemaSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing submission to check ownership
	existingSubmission, err := h.schemaService.GetSchemaSubmission(submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if submission belongs to the user
		if existingSubmission.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
			return
		}
	}

	var req models.UpdateSchemaSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	submission, err := h.schemaService.UpdateSchemaSubmission(submissionId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeSchemaSubmissions), &existingSubmission.SubmissionID, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeSchemaSubmissions), &submission.SubmissionID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// Schema handlers

// GetAllSchemas handles GET /api/v1/schemas
func (h *V1Handler) GetAllSchemas(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionReadSchema) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// For non-admin users, filter results to only their own schemas
	var filteredMemberId *string
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberId, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		filteredMemberId = &userMemberId
	} else {
		// Admin can specify memberId or see all
		filteredMemberId = &memberId
	}

	schemas, err := h.schemaService.GetSchemas(filteredMemberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := models.CollectionResponse{
		Items: schemas,
		Count: len(schemas),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetSchema handles GET /api/v1/schemas/{schemaId}
func (h *V1Handler) GetSchema(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionReadSchema) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	schema, err := h.schemaService.GetSchema(schemaId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if schema belongs to the user
		if schema.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	utils.RespondWithSuccess(w, http.StatusOK, schema)
}

// CreateSchema handles POST /api/v1/schemas
func (h *V1Handler) CreateSchema(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionCreateSchema) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req models.CreateSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// For non-admin users, ensure they can only create schemas for themselves
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Set the member ID to the authenticated user's member ID
		req.MemberID = userMemberID
	}

	schema, err := h.schemaService.CreateSchema(&req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), nil, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &schema.SchemaID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, schema)
}

// UpdateSchema handles PUT /api/v1/schemas/{schemaId}
func (h *V1Handler) UpdateSchema(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionUpdateSchema) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing schema to check ownership
	existingSchema, err := h.schemaService.GetSchema(schemaId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if schema belongs to the user
		if existingSchema.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
			return
		}
	}

	var req models.UpdateSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	schema, err := h.schemaService.UpdateSchema(schemaId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &existingSchema.SchemaID, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &schema.SchemaID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, schema)
}

// Schema policy metadata handlers

// authorizeSchemaAccess checks that the authenticated user has the permission and,
// for non-admin users, owns the schema. It writes the error response and returns
// false when access is denied.
func (h *V1Handler) authorizeSchemaAccess(w http.ResponseWriter, r *http.Request, schemaId string, permission models.Permission) bool {
	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return false
	}

	// Check permission
	if !user.HasPermission(permission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return false
	}

	schema, err := h.schemaService.GetSchema(schemaId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return false
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return false
		}

		// Check if schema belongs to the user
		if schema.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return false
		}
	}

	return true
}

// ListSchemaPolicyMetadata handles GET /api/v1/schemas/{schemaId}/policy-metadata
func (h *V1Handler) ListSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	if !h.authorizeSchemaAccess(w, r, schemaId, models.PermissionReadSchema) {
		return
	}

	list, err := h.schemaService.ListPolicyMetadata(schemaId)
	if err != nil {
		respondWithPolicyMetadataError(w, err)
		return
	}

	response := models.CollectionResponse{
		Items: list.Records,
		Count: len(list.Records),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// PatchSchemaPolicyMetadata handles PATCH /api/v1/schemas/{schemaId}/policy-metadata/{id}
func (h *V1Handler) PatchSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	if !h.authorizeSchemaAccess(w, r, schemaId, models.PermissionUpdateSchema) {
		return
	}

	var req models.PolicyMetadataPatchRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.IsEmpty() {
		utils.RespondWithError(w, http.StatusBadRequest, "at least one editable field is required")
		return
	}

	record, err := h.schemaService.PatchPolicyMetadata(schemaId, id, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &schemaId, string(models.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &schemaId, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, record)
}

// DeleteSchemaPolicyMetadata handles DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}
func (h *V1Handler) DeleteSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	if !h.authorizeSchemaAccess(w, r, schemaId, models.PermissionUpdateSchema) {
		return
	}

	if err := h.schemaService.DeletePolicyMetadata(schemaId, id); err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &schemaId, string(models.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &schemaId, string(models.AuditStatusSuccess))

	w.WriteHeader(http.StatusNoContent)
}

// RevokeSchemaPolicyAllowListEntry handles DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}/allowlist/{applicationId}
func (h *V1Handler) RevokeSchemaPolicyAllowListEntry(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	applicationId := r.PathValue("applicationId")
	if !h.authorizeSchemaAccess(w, r, schemaId, models.PermissionUpdateSchema) {
		return
	}

	if err := h.schemaService.RevokeAllowListEntry(schemaId, id, applicationId); err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &schemaId, string(models.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeSchemas), &schemaId, string(models.AuditStatusSuccess))

	w.WriteHeader(http.StatusNoContent)
}

// respondWithPolicyMetadataError maps policy metadata errors onto a response.
// Client errors reported by the PDP are passed through, while PDP failures and
// unreachable PDPs are reported as a bad gateway.
func respondWithPolicyMetadataError(w http.ResponseWriter, err error) {
	if errors.Is(err, services.ErrPolicyMetadataNotFound) {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	var pdpErr *services.PDPError
	if errors.As(err, &pdpErr) {
		switch pdpErr.StatusCode {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
			utils.RespondWithError(w, pdpErr.StatusCode, pdpErr.Message)
			return
		}
	}

	slog.Error("Policy metadata request to PDP failed", "error", err)
	utils.RespondWithError(w, http.StatusBadGateway, "Policy decision point request failed")
}

// Application submission handlers

// GetAllApplicationSubmissions handles GET /api/v1/application-submissions
func (h *V1Handler) GetAllApplicationSubmissions(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")
	statusFilter := r.URL.Query()["status"]

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionReadApplicationSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var finalMemberId *string = &memberId

	// For non-admin users, force filtering to their own submissions only
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Force the memberId to the authenticated user's member ID
		finalMemberId = &userMemberID
	}

	submissions, err := h.applicationService.GetApplicationSubmissions(r.Context(), finalMemberId, &statusFilter)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := models.CollectionResponse{
		Items: submissions,
		Count: len(submissions),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetApplicationSubmission handles GET /api/v1/application-submissions/{submissionId}
func (h *V1Handler) GetApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionReadApplicationSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	submission, err := h.applicationService.GetApplicationSubmission(r.Context(), submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if submission belongs to the user
		if submission.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// CreateApplicationSubmission handles POST /api/v1/application-submissions
func (h *V1Handler) CreateApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionCreateApplicationSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req models.CreateApplicationSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// For non-admin users, ensure they can only create submissions for themselves
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// If MemberID is provided, validate ownership
		if req.MemberID != "" {
			// Check if the provided MemberID belongs to the authenticated user
			if req.MemberID != userMemberID {
				utils.RespondWithError(w, http.StatusForbidden, "Access denied: cannot create submission for another user")
				return
			}
		} else {
			// If no MemberID provided, set it to the authenticated user's member ID
			req.MemberID = userMemberID
		}
	}

	submission, err := h.applicationService.CreateApplicationSubmission(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeApplicationSubmissions), nil, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeApplicationSubmissions), &submission.SubmissionID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, submission)
}

// UpdateApplicationSubmission handles PUT /api/v1/application-submissions/{submissionId}
func (h *V1Handler) UpdateApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionUpdateApplicationSubmission) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing submission to check ownership
	existingSubmission, err := h.applicationService.GetApplicationSubmission(r.Context(), submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, "Application submission not found")
		return
	}

	// For non-admin users, check ownership before updating
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if submission belongs to the user
		if existingSubmission.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	var req models.UpdateApplicationSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	submission, err := h.applicationService.UpdateApplicationSubmission(r.Context(), submissionId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeApplicationSubmissions), &existingSubmission.SubmissionID, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeApplicationSubmissions), &submission.SubmissionID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// Application handlers

// GetAllApplications handles GET /api/v1/applications
func (h *V1Handler) GetAllApplications(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	var filteredMemberId *string
	if user.HasPermission(models.PermissionReadAllApplications) {
		// Admin/System can use provided filters or see all
		filteredMemberId = &memberId
	} else if user.HasPermission(models.PermissionReadApplication) {
		// Regular users can only see their own applications
		// Get member ID for the authenticated user (cached)
		userMemberId, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		filteredMemberId = &userMemberId
	} else {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	applications, err := h.applicationService.GetApplications(r.Context(), filteredMemberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := models.CollectionResponse{
		Items: applications,
		Count: len(applications),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetApplication handles GET /api/v1/applications/{applicationId}
func (h *V1Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionReadApplication) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	application, err := h.applicationService.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if application belongs to the user
		if application.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
			return
		}
	}

	utils.RespondWithSuccess(w, http.StatusOK, application)
}

// GetApplicationIdByClientId handles GET /internal/api/v1/applications
func (h *V1Handler) GetApplicationIdByClientId(w http.ResponseWriter, r *http.Request) {
	idpClientId := r.URL.Query().Get("idpClientId")
	if idpClientId == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "idpClientId query parameter is required")
		return
	}

	applicationId, err := h.applicationService.GetApplicationIdByIdpClientId(r.Context(), idpClientId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}
	utils.RespondWithSuccess(w, http.StatusOK, applicationId)
}

// CreateApplication handles POST /api/v1/applications
func (h *V1Handler) CreateApplication(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionCreateApplication) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	var req models.CreateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// For non-admin users, ensure they can only create applications for themselves
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Set the member ID to the authenticated user's member ID
		req.MemberID = userMemberID
	}

	application, err := h.applicationService.CreateApplication(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeApplications), nil, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeApplications), &application.ApplicationID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, application)
}

// UpdateApplication handles PUT /api/v1/applications/{applicationId}
func (h *V1Handler) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission
	if !user.HasPermission(models.PermissionUpdateApplication) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing application to check ownership
	existingApplication, err := h.applicationService.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if application belongs to the user
		if existingApplication.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
			return
		}
	}

	var req models.UpdateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	application, err := h.applicationService.UpdateApplication(r.Context(), applicationId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeApplications), &existingApplication.ApplicationID, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeApplications), &application.ApplicationID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, application)
}

// UpdateApplicationPolicy handles PUT /api/v1/applications/{applicationId}/policy
func (h *V1Handler) UpdateApplicationPolicy(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	// Get authenticated user
	user, err := middleware.GetUserFromRequest(r)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Check permission - policy updates share the application:update permission
	if !user.HasPermission(models.PermissionUpdateApplication) {
		utils.RespondWithError(w, http.StatusForbidden, "Insufficient permissions")
		return
	}

	// Get existing application to check ownership
	existingApplication, err := h.applicationService.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// For non-admin users, check ownership
	if !user.IsAdmin() {
		// Get member ID for the authenticated user (cached)
		userMemberID, err := h.getUserMemberID(r, user)
		if err != nil {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// Check if application belongs to the user
		if existingApplication.MemberID != userMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
			return
		}
	}

	var req models.UpdateApplicationPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	application, err := h.applicationService.UpdateApplicationPolicy(r.Context(), applicationId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(models.ResourceTypeApplications), &existingApplication.ApplicationID, string(models.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(models.ResourceTypeApplications), &application.ApplicationID, string(models.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, application)
}
