package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gov-dx-sandbox/portal-backend/idp"
	"github.com/gov-dx-sandbox/portal-backend/idp/idpfactory"
	"github.com/gov-dx-sandbox/portal-backend/shared/utils"
	"github.com/gov-dx-sandbox/portal-backend/v1/middleware"
	"github.com/gov-dx-sandbox/portal-backend/v1/models"
	"github.com/gov-dx-sandbox/portal-backend/v1/services"

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
	asgScopesEnv := os.Getenv("ASGARDEO_SCOPES")
	var scopes []string
	if asgScopesEnv != "" {
		// Split by space to handle multiple scopes
		scopes = strings.Fields(asgScopesEnv)
	}
	// Create the NewIdpProvider
	baseURL := os.Getenv("ASGARDEO_BASE_URL")
	jwksURL := os.Getenv("ASGARDEO_JWKS_URL")
	issuerURL := os.Getenv("ASGARDEO_ISSUER")
	tokenURL := os.Getenv("ASGARDEO_TOKEN_URL")

	if baseURL == "" {
		if jwksURL != "" && (issuerURL != "" || tokenURL != "") {
			if issuerURL != "" {
				baseURL = issuerURL
			} else {
				baseURL = tokenURL
			}
		}
	}

	clientID := os.Getenv("ASGARDEO_CLIENT_ID")
	clientSecret := os.Getenv("ASGARDEO_CLIENT_SECRET")

	if baseURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("failed to create IDP provider: missing required environment variables (ASGARDEO_BASE_URL, or ASGARDEO_JWKS_URL and ASGARDEO_ISSUER/ASGARDEO_TOKEN_URL, along with ASGARDEO_CLIENT_ID and ASGARDEO_CLIENT_SECRET)")
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

	pdpServiceURL := os.Getenv("CHOREO_PDP_CONNECTION_SERVICEURL")
	if pdpServiceURL == "" {
		return nil, fmt.Errorf("CHOREO_PDP_CONNECTION_SERVICEURL environment variable not set")
	}

	pdpServiceAPIKey := os.Getenv("CHOREO_PDP_CONNECTION_CHOREOAPIKEY")
	if pdpServiceAPIKey == "" {
		return nil, fmt.Errorf("CHOREO_PDP_CONNECTION_CHOREOAPIKEY environment variable not set")
	}

	pdpService := services.NewPDPService(pdpServiceURL, pdpServiceAPIKey)
	slog.Info("PDP Service URL", "url", pdpServiceURL)

	return &V1Handler{
		memberService:      memberService,
		schemaService:      services.NewSchemaService(db, pdpService),
		applicationService: services.NewApplicationService(db, pdpService, idpProvider),
	}, nil
}

// SetupV1Routes configures all V1 API routes
func (h *V1Handler) SetupV1Routes(mux *http.ServeMux) {
	// Schema routes
	mux.Handle("/api/v1/schemas", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleSchemas)))
	mux.Handle("/api/v1/schemas/", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleSchemas)))

	// SchemaSubmission routes
	mux.Handle("/api/v1/schema-submissions", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleSchemaSubmissions)))
	mux.Handle("/api/v1/schema-submissions/", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleSchemaSubmissions)))

	// Application routes
	mux.Handle("/internal/api/v1/applications", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleInternalApplications)))
	mux.Handle("/internal/api/v1/applications/", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleInternalApplications)))
	mux.Handle("/api/v1/applications", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleApplications)))
	mux.Handle("/api/v1/applications/", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleApplications)))

	// ApplicationSubmission routes
	mux.Handle("/api/v1/application-submissions", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleApplicationSubmissions)))
	mux.Handle("/api/v1/application-submissions/", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleApplicationSubmissions)))

	// Member routes
	mux.Handle("/api/v1/members", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleMembers)))
	mux.Handle("/api/v1/members/", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handleMembers)))
}

// handleMembers handles member-related routes
func (h *V1Handler) handleMembers(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/members")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Handle collection endpoint: GET /api/v1/members and POST /api/v1/members
	if len(parts) == 1 && parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			idpUserId := r.URL.Query().Get("idpUserId")
			email := r.URL.Query().Get("email")
			h.getAllMembers(w, r, &idpUserId, &email)
		case http.MethodPost:
			h.createMember(w, r)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	if len(parts) < 1 || parts[0] == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Member ID is required")
		return
	}

	memberId := parts[0]

	// Handle base member endpoint: GET /api/v1/members/:memberId and PUT /api/v1/members/:memberId
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getMember(w, r, memberId)
		case http.MethodPut:
			h.updateMember(w, r, memberId)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	utils.RespondWithError(w, http.StatusNotFound, "Endpoint not found")
}

// handleSchemas handles schema-related routes
func (h *V1Handler) handleSchemas(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/schemas")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Handle collection endpoint: GET /api/v1/schemas and POST /api/v1/schemas
	if len(parts) == 1 && parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			memberId := r.URL.Query().Get("memberId")
			h.getAllSchemas(w, r, &memberId)
		case http.MethodPost:
			h.createSchema(w, r)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}
	if len(parts) < 1 || parts[0] == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Schema ID is required")
		return
	}
	schemaId := parts[0]

	// Handle specific schema endpoint: GET /api/v1/schemas/:schemaId and PUT /api/v1/schemas/:schemaId
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getSchema(w, r, schemaId)
		case http.MethodPut:
			h.updateSchema(w, r, schemaId)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	utils.RespondWithError(w, http.StatusNotFound, "Endpoint not found")
}

// handleSchemaSubmissions handles schema submission-related routes
func (h *V1Handler) handleSchemaSubmissions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/schema-submissions")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Handle collection endpoint: GET /api/v1/schema-submissions and POST /api/v1/schema-submissions
	if len(parts) == 1 && parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			status := r.URL.Query()["status"]
			memberId := r.URL.Query().Get("memberId")
			h.getAllSchemaSubmissions(w, r, &memberId, &status)
		case http.MethodPost:
			h.createSchemaSubmission(w, r, nil)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}
	if len(parts) < 1 || parts[0] == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Submission ID is required")
		return
	}
	submissionId := parts[0]
	// Handle specific schema submission endpoint: GET /api/v1/schema-submissions/:submissionId and PUT /api/v1/schema-submissions/:submissionId
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getSchemaSubmission(w, r, submissionId)
		case http.MethodPut:
			h.updateSchemaSubmission(w, r, submissionId)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	utils.RespondWithError(w, http.StatusNotFound, "Endpoint not found")
}

// handleInternalApplications handles internal application-related routes
func (h *V1Handler) handleInternalApplications(w http.ResponseWriter, r *http.Request) {
	// Only internal operation currently needed is getApplicationId by IdpClientId
	if r.URL.Path != "/internal/api/v1/applications" && r.URL.Path != "/internal/api/v1/applications/" {
		utils.RespondWithError(w, http.StatusNotFound, "Endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.getApplicationIdByClientId(w, r)
	default:
		utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleApplications handles application-related routes
func (h *V1Handler) handleApplications(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/applications")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Handle collection endpoint: GET /api/v1/applications and POST /api/v1/applications
	if len(parts) == 1 && parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			memberId := r.URL.Query().Get("memberId")
			h.getAllApplications(w, r, &memberId)
		case http.MethodPost:
			h.createApplication(w, r)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}
	if len(parts) < 1 || parts[0] == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Application ID is required")
		return
	}

	applicationId := parts[0]
	// Handle specific application endpoint: GET /api/v1/applications/:applicationId and PUT /api/v1/applications/:applicationId
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getApplication(w, r, applicationId)
		case http.MethodPut:
			h.updateApplication(w, r, applicationId)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	utils.RespondWithError(w, http.StatusNotFound, "Endpoint not found")
}

// handleApplicationSubmissions handles application submission-related routes
func (h *V1Handler) handleApplicationSubmissions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/application-submissions")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Handle collection endpoint: GET /api/v1/application-submissions and POST /api/v1/application-submissions
	if len(parts) == 1 && parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			status := r.URL.Query()["status"]
			memberId := r.URL.Query().Get("memberId")
			h.getAllApplicationSubmissions(w, r, &memberId, &status)
		case http.MethodPost:
			h.createApplicationSubmission(w, r, nil)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	if len(parts) < 1 || parts[0] == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Submission ID is required")
		return
	}

	submissionId := parts[0]
	// Handle specific application submission endpoint: GET /api/v1/application-submissions/:submissionId and PUT /api/v1/application-submissions/:submissionId
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getApplicationSubmission(w, r, submissionId)
		case http.MethodPut:
			h.updateApplicationSubmission(w, r, submissionId)
		default:
			utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}
	utils.RespondWithError(w, http.StatusNotFound, "Endpoint not found")
}

// Member handlers
func (h *V1Handler) createMember(w http.ResponseWriter, r *http.Request) {
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

func (h *V1Handler) updateMember(w http.ResponseWriter, r *http.Request, memberId string) {
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

func (h *V1Handler) getMember(w http.ResponseWriter, r *http.Request, memberId string) {
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

func (h *V1Handler) getAllMembers(w http.ResponseWriter, r *http.Request, idpUserId *string, email *string) {
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
		filteredIdpUserId = idpUserId
		// Note: We still accept email parameter from query but don't use it
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

// Schema handlers
func (h *V1Handler) getAllSchemaSubmissions(w http.ResponseWriter, r *http.Request, memberId *string, statusFilter *[]string) {
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
		filteredMemberId = memberId
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

	submissions, err := h.schemaService.GetSchemaSubmissions(filteredMemberId, statusFilter)
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

func (h *V1Handler) getSchemaSubmission(w http.ResponseWriter, r *http.Request, submissionId string) {
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

func (h *V1Handler) createSchemaSubmission(w http.ResponseWriter, r *http.Request, memberId *string) {
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

	// If memberId is provided from URL parameter, use it
	if memberId != nil {
		req.MemberID = *memberId
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

func (h *V1Handler) updateSchemaSubmission(w http.ResponseWriter, r *http.Request, submissionId string) {
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

func (h *V1Handler) getAllSchemas(w http.ResponseWriter, r *http.Request, memberId *string) {
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
		filteredMemberId = memberId
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

func (h *V1Handler) getSchema(w http.ResponseWriter, r *http.Request, schemaId string) {
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

func (h *V1Handler) createSchema(w http.ResponseWriter, r *http.Request) {
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

func (h *V1Handler) updateSchema(w http.ResponseWriter, r *http.Request, schemaId string) {
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

// Application handlers
func (h *V1Handler) getAllApplicationSubmissions(w http.ResponseWriter, r *http.Request, memberId *string, statusFilter *[]string) {
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

	var finalMemberId *string = memberId

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

	submissions, err := h.applicationService.GetApplicationSubmissions(r.Context(), finalMemberId, statusFilter)
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

func (h *V1Handler) getApplicationSubmission(w http.ResponseWriter, r *http.Request, submissionId string) {
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

func (h *V1Handler) createApplicationSubmission(w http.ResponseWriter, r *http.Request, memberId *string) {
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

	// If memberId is provided from URL parameter, use it
	if memberId != nil {
		req.MemberID = *memberId
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

func (h *V1Handler) updateApplicationSubmission(w http.ResponseWriter, r *http.Request, submissionId string) {
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

func (h *V1Handler) getAllApplications(w http.ResponseWriter, r *http.Request, memberId *string) {
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
		filteredMemberId = memberId
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

func (h *V1Handler) getApplication(w http.ResponseWriter, r *http.Request, applicationId string) {
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

func (h *V1Handler) getApplicationIdByClientId(w http.ResponseWriter, r *http.Request) {
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

func (h *V1Handler) createApplication(w http.ResponseWriter, r *http.Request) {
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

func (h *V1Handler) updateApplication(w http.ResponseWriter, r *http.Request, applicationId string) {
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
