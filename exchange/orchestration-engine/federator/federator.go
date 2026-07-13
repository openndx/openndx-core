package federator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	auditpkg "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNDX/openndx-core/exchange/shared/monitoring"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/auth"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/configs"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/consent"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/internals/errors"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/middleware"
	auth2 "github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/pkg/auth"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/pkg/graphql"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/policy"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/provider"
	"github.com/google/uuid"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/graphql-go/graphql/language/source"
	"golang.org/x/oauth2/clientcredentials"
)

// Federator struct that includes all the context needed for federation.
type Federator struct {
	Configs         *configs.Config
	ProviderHandler *provider.Handler
	Client          *http.Client
	Schema          *ast.Document
	SchemaService   interface{}          // Will be *services.SchemaService, using interface{} to avoid circular import
	TokenValidator  *auth.TokenValidator // Cached validator for JWT token signature verification
}

type FederationServiceAST struct {
	ServiceKey string
	SchemaID   string
	QueryAst   *ast.Document
}

type federationServiceRequest struct {
	ServiceKey     string
	SchemaID       string
	GraphQLRequest graphql.Request
}

type federationRequest struct {
	// Define fields as needed
	FederationServiceRequest []*federationServiceRequest
}

type ProviderResponse struct {
	ServiceKey string
	Response   graphql.Response `json:"response"`
}

type FederationResponse struct {
	ServiceKey string              `json:"ProviderKey"`
	Responses  []*ProviderResponse `json:"responses"`
}

// GetProviderResponse Returns the specific provider response by service key
func (f *FederationResponse) GetProviderResponse(providerKey string) *ProviderResponse {
	for _, resp := range f.Responses {
		if resp.ServiceKey == providerKey {
			return resp
		}
	}
	return nil
}

// createErrorResponse creates a GraphQL error response with the given message and optional extensions
func createErrorResponse(message string, extensions map[string]interface{}) graphql.Response {
	errorMap := map[string]interface{}{
		"message": message,
	}
	if extensions != nil {
		errorMap["extensions"] = extensions
	}
	return graphql.Response{
		Data: nil,
		Errors: []interface{}{
			errorMap,
		},
	}
}

// createErrorResponseWithCode creates a GraphQL error response with a message and error code
func createErrorResponseWithCode(message string, code string) graphql.Response {
	return createErrorResponse(message, map[string]interface{}{
		"code": code,
	})
}

// Initialize sets up the Federator with providers and an HTTP client.
// Returns error if critical configuration is invalid (fail-fast approach).
// The provided context controls the lifecycle of background operations (e.g., JWKS auto-refresh).
func Initialize(ctx context.Context, configs *configs.Config, providerHandler *provider.Handler, schemaService interface{}) (*Federator, error) {
	federator := &Federator{
		ProviderHandler: providerHandler,
		SchemaService:   schemaService,
		Configs:         configs,
	}

	// Validate JWT configuration based on trustUpstream setting
	// If trustUpstream is false, we MUST have a valid TokenValidator
	if !configs.TrustUpstream {
		// When not trusting upstream, JWT signature verification is required
		if configs.JWT.JwksUrl == "" {
			return nil, fmt.Errorf("fatal configuration error: trustUpstream is false but JWT.JwksUrl is not configured - cannot verify token signatures")
		}

		// Attempt to create TokenValidator with the configured JWKS URL and auto-refresh
		validator, err := auth.NewTokenValidator(ctx, configs.JWT.JwksUrl, configs.Environment)
		if err != nil {
			return nil, fmt.Errorf("fatal configuration error: failed to initialize TokenValidator with JWKS URL %s: %w", configs.JWT.JwksUrl, err)
		}

		federator.TokenValidator = validator
		logger.Log.Info("TokenValidator initialized successfully with auto-refresh", "jwksUrl", configs.JWT.JwksUrl, "trustUpstream", false)
	} else {
		// When trusting upstream, TokenValidator is optional (may still be used for additional validation)
		if configs.JWT.JwksUrl != "" {
			validator, err := auth.NewTokenValidator(ctx, configs.JWT.JwksUrl, configs.Environment)
			if err != nil {
				logger.Log.Warn("Failed to initialize TokenValidator (non-fatal with trustUpstream=true)", "error", err, "jwksUrl", configs.JWT.JwksUrl)
			} else {
				federator.TokenValidator = validator
				logger.Log.Info("TokenValidator initialized successfully with auto-refresh", "jwksUrl", configs.JWT.JwksUrl, "trustUpstream", true)
			}
		} else {
			logger.Log.Info("TokenValidator not configured (trustUpstream=true, no JWKS URL provided)")
		}
	}

	// Initialize with providers from config if available
	if configs.Providers != nil {
		for _, p := range configs.Providers {
			// Convert ProviderConfig to Provider
			providerInstance := &provider.Provider{
				ServiceUrl: p.ProviderURL,
				ServiceKey: p.ProviderKey,
				SchemaID:   p.SchemaID,
				Auth:       p.Auth,
			}

			if p.Auth != nil && p.Auth.Type == auth2.AuthTypeOAuth2 {
				providerInstance.OAuth2Config = &clientcredentials.Config{
					ClientID:     p.Auth.ClientID,
					ClientSecret: p.Auth.ClientSecret,
					TokenURL:     p.Auth.TokenURL,
				}
			}

			// print service url
			logger.Log.Info("Adding Provider from the Config File", "Provider Key", p.ProviderKey, "Provider Url", p.ProviderURL)
			providerHandler.AddProvider(providerInstance)
		}
	} else {
		logger.Log.Info("No Providers found in the Config File")
	}

	// Initialize HTTP client with timeout and connection pooling
	federator.Client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
		},
	}

	return federator, nil
}

// FederateQuery takes a raw GraphQL query, splits it into sub-queries for each service,
// sends them to the respective providers, and merges the responses.
func (f *Federator) FederateQuery(ctx context.Context, request graphql.Request, consumerInfo *auth.ConsumerAssertion) graphql.Response {
	// Ensure traceID is in context (should already be set by monitoring.TraceIDMiddleware, but ensure it)
	traceID := monitoring.GetTraceIDFromContext(ctx)
	if traceID == "" {
		traceID = uuid.New().String()
		ctx = monitoring.WithTraceID(ctx, traceID)
	}

	// Log orchestration request received event
	// Update context with traceID if one was generated
	ctx = f.logOrchestrationRequestReceived(ctx, consumerInfo.ApplicationID, request.Query)

	// Convert the query string into its ast
	src := source.NewSource(&source.Source{
		Body: []byte(request.Query),
		Name: "Query",
	})

	doc, err := parser.Parse(parser.ParseParams{Source: src})
	if err != nil {
		logger.Log.Error("Failed to parse query", "Error", err)
	}

	// Get schema document from database or config
	var schema *ast.Document

	// First try to get from database if schema service is available
	if f.SchemaService != nil {
		// Use reflection to call GetActiveSchema method
		schemaServiceValue := reflect.ValueOf(f.SchemaService)
		if schemaServiceValue.IsValid() && !schemaServiceValue.IsNil() {
			getActiveSchemaMethod := schemaServiceValue.MethodByName("GetActiveSchema")
			if getActiveSchemaMethod.IsValid() {
				results := getActiveSchemaMethod.Call([]reflect.Value{})
				if len(results) >= 2 && !results[1].IsNil() {
					// Error occurred
					logger.Log.Warn("Failed to get active schema from database", "Error", results[1].Interface())
				} else if len(results) >= 1 && !results[0].IsNil() {
					// Got schema from database
					schemaRecord := results[0].Interface()
					// Extract SDL from schema record using reflection
					schemaRecordValue := reflect.ValueOf(schemaRecord)
					// If it's a pointer, dereference it
					if schemaRecordValue.Kind() == reflect.Ptr {
						schemaRecordValue = schemaRecordValue.Elem()
					}
					sdlField := schemaRecordValue.FieldByName("SDL")
					if sdlField.IsValid() && sdlField.Kind() == reflect.String {
						sdlString := sdlField.String()
						src := source.NewSource(&source.Source{
							Body: []byte(sdlString),
							Name: "ActiveSchema",
						})
						schema, err = parser.Parse(parser.ParseParams{Source: src})
						if err != nil {
							logger.Log.Error("Failed to parse active schema from database", "Error", err)
							schema = nil
						}
					}
				}
			}
		}
	} else {
		logger.Log.Info("SchemaService is nil, skipping database schema lookup")
	}

	// Fallback to config if no schema from database
	if schema == nil && f.Configs.Schema != nil {
		schema, err = f.Configs.GetSchemaDocument()
		if err != nil {
			logger.Log.Warn("Failed to get schema from config", "Error", err)
			schema = nil
		}
	}

	// Final fallback to schema.graphql file if no schema from database or config
	if schema == nil {
		logger.Log.Info("No schema found in database or config, attempting to load schema.graphql file")
		schema, err = f.loadSchemaFromFile()
		if err != nil {
			logger.Log.Error("Failed to load schema from file", "Error", err)
			return graphql.Response{
				Data: nil,
				Errors: []interface{}{
					&graphql.JSONError{
						Message: "No active schema found. Please create and activate a schema using the schema management API first, or ensure schema.graphql file exists.",
					},
				},
			}
		}
	}

	// Collect the directives from the query
	schemaCollection, err := ProviderSchemaCollector(schema, doc)
	if err != nil {
		logger.Log.Error("Failed to collect provider schema", "Error", err)
		return graphql.Response{
			Data: nil,
			Errors: []interface{}{
				err.(*graphql.JSONError),
			},
		}
	}

	// Safely get argument mapping with nil check
	var argMapping []*graphql.ArgMapping
	if f.Configs.ArgMapping != nil {
		argMapping = f.Configs.ArgMapping
	}

	requiredArguments := FindRequiredArguments(schemaCollection.ProviderFieldMap, argMapping)

	extractedArgs := ExtractRequiredArguments(requiredArguments, schemaCollection.Arguments)

	// check whether there are variables in the request
	if request.Variables != nil {
		// if there are variables, replace the argument values with the variable values
		PushVariablesFromVariableDefinition(request, extractedArgs, schemaCollection.VariableDefinitions)
	}

	// Safely initialize PDP and CE clients with nil checks
	var pdpClient *policy.PdpClient
	var ceClient *consent.CEServiceClient

	if f.Configs.PdpConfig.ClientURL != "" {
		pdpClient = policy.NewPdpClient(f.Configs.PdpConfig.ClientURL)
	}
	if f.Configs.CeConfig.ClientURL != "" {
		ceClient = consent.NewCEServiceClient(f.Configs.CeConfig.ClientURL)
	}

	// Check if PDP client is available before making request
	var pdpResponse *policy.PdpResponse
	if pdpClient == nil {
		logger.Log.Warn("PDP client not available, skipping policy check")
		// Continue without PDP check - this allows the system to work without PDP
	} else {
		pdpRequest := &policy.PdpRequest{
			AppId: consumerInfo.ApplicationID,
		}

		requiredFields := make([]policy.RequiredField, 0)

		for _, field := range *schemaCollection.ProviderFieldMap {
			requiredFields = append(requiredFields, policy.RequiredField{
				SchemaID:  field.SchemaId,
				FieldName: field.FieldPath,
			})
		}

		pdpRequest.RequiredFields = requiredFields

		pdpResponse, err = pdpClient.MakePdpRequest(ctx, pdpRequest)

		// Log policy check audit event
		// Update context with traceID if one was generated
		ctx = f.logPolicyCheck(ctx, consumerInfo.ApplicationID, pdpRequest, pdpResponse, err)

		if err != nil {
			logger.Log.Error("PDP request failed", "error", err)
			return createErrorResponseWithCode(fmt.Sprintf("Authorization check failed: %v", err), errors.CodePDPError)
		}

		if pdpResponse == nil {
			logger.Log.Error("Failed to get response from PDP")
			return createErrorResponseWithCode("No response from authorization service", errors.CodePDPNoResponse)
		}

		// Log PDP decision for audit trail
		logger.Log.Info("PDP decision received",
			"authorized", pdpResponse.AppAuthorized,
			"consentRequired", pdpResponse.AppRequiresOwnerConsent,
			"unauthorizedFieldsCount", len(pdpResponse.UnauthorizedFields),
			"expiredFieldsCount", len(pdpResponse.ExpiredFields))

		if !pdpResponse.AppAuthorized {
			logger.Log.Info("Request not authorized by PDP",
				"unauthorizedFields", pdpResponse.UnauthorizedFields)
			return createErrorResponse("Access denied", map[string]interface{}{
				"code":               errors.CodePDPNotAllowed,
				"unauthorizedFields": pdpResponse.UnauthorizedFields,
			})
		}

		if pdpResponse.AppAccessExpired {
			logger.Log.Info("Application access expired",
				"expiredFields", pdpResponse.ExpiredFields)
			return createErrorResponse("Access expired", map[string]interface{}{
				"code":          errors.CodePDPNotAllowed,
				"expiredFields": pdpResponse.ExpiredFields,
			})
		}
	}

	// Check for Data Owner ID in extracted arguments
	var dataOwnerID string
	if len(extractedArgs) == 0 {
		logger.Log.Info("Data Owner ID argument is missing: extractedArgs is empty or nil")
		return createErrorResponseWithCode("Data Owner ID argument is missing", errors.CodeMissingEntityIdentifier)
	}
	val := extractedArgs[0].Value.GetValue()
	if s, ok := val.(string); ok {
		dataOwnerID = s
	} else {
		logger.Log.Error("CitizenID is not a string", "value", val)
		dataOwnerID = ""
	}
	if dataOwnerID == "" {
		logger.Log.Info("Data Owner ID argument is missing or invalid")
		return createErrorResponseWithCode("Data Owner ID argument is missing or invalid", errors.CodeMissingEntityIdentifier)
	}

	// Handle consent check if consent is required
	if pdpResponse != nil && pdpResponse.AppRequiresOwnerConsent {
		logger.Log.Info("Consent required for fields",
			"fieldsCount", len(pdpResponse.ConsentRequiredFields),
			"fields", pdpResponse.ConsentRequiredFields)

		// Validate PDP response
		if len(pdpResponse.ConsentRequiredFields) == 0 {
			logger.Log.Error("PDP indicates consent required but no fields specified")
			return createErrorResponseWithCode("Invalid PDP response: consent required but no fields specified", errors.CodePDPError)
		}

		// Check if CE client is available
		if ceClient == nil {
			logger.Log.Warn("CE client not available, skipping consent check")
			return createErrorResponseWithCode("Consent required but consent engine not available", errors.CodeCEError)
		}

		ownerEmail := dataOwnerID // assuming dataOwnerID is ownerEmail for this example

		// Map PDP response fields to Consent Engine request with all metadata
		fields := make([]consent.ConsentField, len(pdpResponse.ConsentRequiredFields))
		for i, f := range pdpResponse.ConsentRequiredFields {
			fields[i].FieldName = f.FieldName
			fields[i].SchemaID = f.SchemaID
			fields[i].DisplayName = f.DisplayName
			fields[i].Description = f.Description

			// Map Owner from PDP response, default to citizen if not provided
			if f.Owner != nil {
				fields[i].Owner = consent.OwnerType(*f.Owner)
			} else {
				fields[i].Owner = consent.OwnerCitizen
			}
		}

		typeRealTime := consent.TypeRealtime
		ceRequest := &consent.CreateConsentRequest{
			AppID: consumerInfo.ApplicationID,
			ConsentRequirement: consent.ConsentRequirement{
				Owner:      consent.OwnerCitizen,
				OwnerID:    ownerEmail,
				OwnerEmail: ownerEmail,
				Fields:     fields,
			},
			ConsentType: &typeRealTime,
		}

		ceResp, err := ceClient.CreateConsent(ctx, ceRequest)

		// Log consent check audit event
		// Update context with traceID if one was generated
		ctx = f.logConsentCheck(ctx, consumerInfo.ApplicationID, ownerEmail, ownerEmail, ceRequest, ceResp, err)

		if err != nil {
			logger.Log.Info("CE request failed", "error", err)
			return createErrorResponseWithCode("CE request failed", errors.CodeCEError)
		}
		if ceResp == nil {
			logger.Log.Error("Failed to get response from CE")
			return createErrorResponseWithCode("Failed to get response from CE", errors.CodeCENoResponse)
		}

		// log the consent response
		logger.Log.Info("Consent Response", "response", ceResp)

		// Check consent status - only proceed if approved
		if ceResp.Status == consent.StatusApproved {
			logger.Log.Info("Consent approved, proceeding with query execution")
		} else {
			// Status is pending or any other non-approved status
			logger.Log.Info("Consent not approved", "status", ceResp.Status)
			return createErrorResponse("Consent not approved", map[string]interface{}{
				"code":             errors.CodeCENotApproved,
				"consentPortalUrl": ceResp.ConsentPortalURL,
				"consentStatus":    ceResp.Status,
			})
		}
	}

	splitRequests, err := QueryBuilder(schemaCollection.ProviderFieldMap, extractedArgs)
	if err != nil {
		logger.Log.Error("Failed to build queries", "Error", err)
		return graphql.Response{
			Data: nil,
			Errors: []interface{}{
				err.(*graphql.JSONError),
			},
		}
	}

	if len(splitRequests) == 0 {
		logger.Log.Info("No valid service queries found in the request")
		return createErrorResponse("No valid service queries found in the request", nil)
	}

	federationRequest := &federationRequest{
		FederationServiceRequest: splitRequests,
	}

	// Inject audit metadata into context
	auditMetadata := &middleware.Metadata{
		ConsumerAppID:    consumerInfo.ApplicationID,
		ProviderFieldMap: convertToAuditFieldRecords(schemaCollection.ProviderFieldMap),
	}
	ctxWithAudit := middleware.NewContextWithMetadata(ctx, auditMetadata)

	responses := f.performFederation(ctxWithAudit, federationRequest)

	// Build schema info map for array-aware processing
	var schemaInfoMap map[string]*SourceSchemaInfo
	if schema != nil {
		schemaInfoMap, err = BuildSchemaInfoMap(schema, doc)
		if err != nil {
			logger.Log.Error("Failed to build schema info map", "Error", err)
		}
	}
	// Error handling is done above in the if block

	// Transform the federated responses back to the original query structure using array-aware processing
	response := AccumulateResponseWithSchemaInfo(doc, responses, schemaInfoMap)

	return response
}

func (f *Federator) performFederation(ctx context.Context, r *federationRequest) *FederationResponse {
	FederationResponse := &FederationResponse{
		Responses: make([]*ProviderResponse, 0, len(r.FederationServiceRequest)),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex // to safely append to FederationResponse.Responses

	for _, request := range r.FederationServiceRequest {
		p, exists := f.ProviderHandler.GetProvider(request.ServiceKey, request.SchemaID)
		if !exists {
			logger.Log.Info("Provider not found", "Provider Key", request.ServiceKey)
			continue
		}

		wg.Add(1)
		go func(req *federationServiceRequest, prov *provider.Provider) {
			defer wg.Done()

			logAudit := func(status string, err error, response *graphql.Response) {
				auditReq := &middleware.FederationServiceRequest{
					ServiceKey:     req.ServiceKey,
					SchemaID:       req.SchemaID,
					GraphQLRequest: req.GraphQLRequest,
				}
				middleware.LogProviderFetch(ctx, req.SchemaID, auditReq, response, err)
			}

			reqBody, err := json.Marshal(req.GraphQLRequest)
			if err != nil {
				logger.Log.Info("Failed to marshal request", "Provider Key", req.ServiceKey, "Error", err)
				logAudit("failure", err, nil)
				return
			}

			response, err := prov.PerformRequest(ctx, reqBody)
			if err != nil {
				logger.Log.Info("Request failed to the Provider", "Provider Key", req.ServiceKey, "Error", err)
				logAudit("failure", err, nil)
				return
			}
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			if err != nil {
				logger.Log.Error("Failed to read response body", "Provider Key", req.ServiceKey, "Error", err)
				logAudit("failure", err, nil)
				return
			}

			var bodyJson graphql.Response
			err = json.Unmarshal(body, &bodyJson)
			if err != nil {
				logger.Log.Error("Failed to unmarshal response", "Provider Key", req.ServiceKey, "Error", err)
				logAudit("failure", err, nil)
				return
			}

			// Log audit event with response
			logAudit("success", nil, &bodyJson)

			// Thread-safe append
			mu.Lock()
			FederationResponse.Responses = append(FederationResponse.Responses, &ProviderResponse{
				ServiceKey: req.ServiceKey,
				Response:   bodyJson,
			})
			mu.Unlock()
		}(request, p)
	}

	wg.Wait()
	return FederationResponse
}

// logOrchestrationRequestReceived logs an ORCHESTRATION_REQUEST_RECEIVED event
// Returns the updated context with traceID to ensure trace correlation
func (f *Federator) logOrchestrationRequestReceived(ctx context.Context, consumerAppID string, query string) context.Context {
	requestMetadata := map[string]interface{}{
		"applicationId": consumerAppID,
		"query":         query,
	}
	// No target for orchestration request received (it's the entry point)
	return middleware.LogRequestReceived(ctx, "DATA_REQUEST", "APPLICATION", consumerAppID, requestMetadata)
}

// logPolicyCheck logs a POLICY_CHECK event from orchestration-engine's perspective
// This is called after making a request to the Policy Decision Point
// Returns the updated context with traceID to ensure trace correlation
func (f *Federator) logPolicyCheck(ctx context.Context, applicationID string, req *policy.PdpRequest, resp *policy.PdpResponse, err error) context.Context {
	targetID := "policy-decision-point"
	status := auditpkg.StatusSuccess
	requestMetadata := make(map[string]interface{})
	responseMetadata := make(map[string]interface{})

	// Populate request metadata
	requestMetadata["applicationId"] = applicationID
	if req != nil {
		requestMetadata["requiredFields"] = req.RequiredFields
	}

	if err != nil {
		status = auditpkg.StatusFailure
		responseMetadata["error"] = err.Error()
	} else if resp != nil {
		responseMetadata["authorized"] = resp.AppAuthorized
		responseMetadata["consentRequired"] = resp.AppRequiresOwnerConsent
		responseMetadata["accessExpired"] = resp.AppAccessExpired
		if !resp.AppAuthorized {
			status = auditpkg.StatusFailure
			responseMetadata["unauthorizedFields"] = resp.UnauthorizedFields
		}
		if resp.AppAccessExpired {
			status = auditpkg.StatusFailure
			responseMetadata["expiredFields"] = resp.ExpiredFields
		}
		if resp.AppRequiresOwnerConsent {
			responseMetadata["consentRequiredFields"] = resp.ConsentRequiredFields
		}
	}

	// Update context with traceID if one was generated
	// Policy Decision Point is a service, so targetType is "SERVICE"
	ctx = middleware.LogAuditEvent(ctx, "POLICY_CHECK", &targetID, "SERVICE", requestMetadata, responseMetadata, status)
	return ctx
}

// logConsentCheck logs a CONSENT_CHECK event from orchestration-engine's perspective
// This is called after making a request to the Consent Engine
// Returns the updated context with traceID to ensure trace correlation
func (f *Federator) logConsentCheck(ctx context.Context, applicationID, ownerEmail, ownerID string, req *consent.CreateConsentRequest, resp *consent.ConsentResponseInternalView, err error) context.Context {
	targetID := "consent-engine"
	status := auditpkg.StatusSuccess
	requestMetadata := make(map[string]interface{})
	responseMetadata := make(map[string]interface{})

	// Populate request metadata from request context
	requestMetadata["applicationId"] = applicationID
	if ownerEmail != "" {
		requestMetadata["ownerEmail"] = ownerEmail
	}
	if ownerID != "" {
		requestMetadata["ownerId"] = ownerID
	}
	if req != nil {
		requestMetadata["fieldsCount"] = len(req.ConsentRequirement.Fields)
	}

	if err != nil {
		status = auditpkg.StatusFailure
		responseMetadata["error"] = err.Error()
	} else if resp != nil {
		responseMetadata["consentId"] = resp.ConsentID
		responseMetadata["status"] = resp.Status
		if resp.ConsentPortalURL != nil {
			responseMetadata["consentPortalUrl"] = *resp.ConsentPortalURL
		}
	}

	// Update context with traceID if one was generated
	// Consent Engine is a service, so targetType is "SERVICE"
	return middleware.LogAuditEvent(ctx, "CONSENT_CHECK", &targetID, "SERVICE", requestMetadata, responseMetadata, status)
}

func (f *Federator) mergeResponses(responses []*ProviderResponse) graphql.Response {
	merged := graphql.Response{
		Data:   make(map[string]interface{}),
		Errors: make([]interface{}, 0),
	}

	for _, resp := range responses {
		if resp.Response.Data != nil {
			for k, v := range resp.Response.Data {
				// wrap v with service key
				merged.Data[resp.ServiceKey] = map[string]interface{}{
					k: v,
				}
			}
		}
		if resp.Response.Errors != nil {
			merged.Errors = append(merged.Errors, resp.Response.Errors...)
		}
	}

	return merged
}

// loadSchemaFromFile loads the schema from schema.graphql file as a fallback
func (f *Federator) loadSchemaFromFile() (*ast.Document, error) {
	// Try to read schema.graphql file from current directory
	schemaData, err := os.ReadFile("schema.graphql")
	if err != nil {
		// Try alternative paths
		alternativePaths := []string{
			"./schema.graphql",
			"../schema.graphql",
			"../../schema.graphql",
		}

		for _, path := range alternativePaths {
			schemaData, err = os.ReadFile(path)
			if err == nil {
				logger.Log.Info("Successfully found schema.graphql at", "path", path)
				break
			}
		}

		if err != nil {
			return nil, fmt.Errorf("could not find schema.graphql file in any expected location: %w", err)
		}
	}

	// Parse the schema file
	src := source.NewSource(&source.Source{
		Body: schemaData,
		Name: "SchemaFile",
	})

	schema, err := parser.Parse(parser.ParseParams{Source: src})
	if err != nil {
		return nil, err
	}

	logger.Log.Info("Successfully loaded schema from schema.graphql file")
	return schema, nil
}
