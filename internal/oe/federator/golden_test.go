package federator_test

// Golden tests for FederateQuery.
//
// These tests pin the observable behaviour of the federation pipeline so the
// refactor in docs/DESIGN-oe-federator-decoupling.md can proceed safely. They
// only touch the federator through its exported API and talk to fake providers,
// PDP and consent engine over HTTP, so they survive internal restructuring.
// When the constructor changes, update newGoldenFederator only.
//
// Each case records a transcript (response, outbound PDP / consent / provider
// requests, audit events) and compares it with testdata/golden/<case>.json.
// Regenerate after an intentional behaviour change with:
//
//	go test ./internal/oe/federator -run TestFederateQueryGolden -update

import (
	"context"
	"crypto"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	auditpkg "github.com/LSFLK/argus/pkg/audit"
	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/configs"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/middleware"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/golden")

const goldenSchemaSDL = `
directive @sourceInfo(
	providerKey: String!
	schemaId: String
	providerField: String!
) on FIELD_DEFINITION

type Query {
	personInfo(nic: String!): PersonInfo
}

type PersonInfo {
	fullName: String @sourceInfo(providerKey: "drp", schemaId: "drp-v1", providerField: "person.fullName")
	address: String @sourceInfo(providerKey: "drp", schemaId: "drp-v1", providerField: "person.permanentAddress")
	birthInfo: BirthInfo
	ownedVehicles: [VehicleInfo] @sourceInfo(providerKey: "dmt", schemaId: "dmt-v1", providerField: "vehicle.getVehicleInfos.data")
}

type BirthInfo {
	date: String @sourceInfo(providerKey: "rgd", schemaId: "rgd-v1", providerField: "getPersonInfo.birthDate")
	place: String @sourceInfo(providerKey: "rgd", schemaId: "rgd-v1", providerField: "getPersonInfo.birthPlace")
}

type VehicleInfo {
	regNo: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-v1", providerField: "vehicle.getVehicleInfos.data.registrationNumber")
	make: String @sourceInfo(providerKey: "dmt", schemaId: "dmt-v1", providerField: "vehicle.getVehicleInfos.data.make")
}
`

var goldenArgMappings = []*graphql.ArgMapping{
	{ProviderKey: "drp", SchemaID: "drp-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "person", TargetArgName: "nic"},
	{ProviderKey: "rgd", SchemaID: "rgd-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "getPersonInfo", TargetArgName: "nic"},
	{ProviderKey: "dmt", SchemaID: "dmt-v1", SourceArgPath: "personInfo-nic", TargetArgPath: "vehicle.getVehicleInfos", TargetArgName: "ownerNic"},
}

type goldenProvider struct {
	key      string
	schemaID string
	data     string
}

var goldenProviders = []goldenProvider{
	{key: "drp", schemaID: "drp-v1", data: `{"person": {"fullName": "Nimal Perera", "permanentAddress": "12 Galle Road, Colombo"}}`},
	{key: "rgd", schemaID: "rgd-v1", data: `{"getPersonInfo": {"birthDate": "1990-01-01", "birthPlace": "Kandy"}}`},
	{key: "dmt", schemaID: "dmt-v1", data: `{"vehicle": {"getVehicleInfos": {"data": [
		{"registrationNumber": "CAB-1234", "make": "Toyota"},
		{"registrationNumber": "WP-5678", "make": "Honda"}
	]}}}`},
}

type goldenCase struct {
	name    string
	request graphql.Request

	// noPDP leaves the PDP URL unset so the policy check is skipped.
	noPDP bool
	// pdpStatus overrides the PDP HTTP status (default 200).
	pdpStatus int
	// pdpResponse overrides the PDP decision (default: authorized, no consent).
	pdpResponse *policy.PdpResponse
	// ceResponse overrides the consent engine reply (default: approved).
	ceResponse *consent.ConsentResponseInternalView
	// providerStatus overrides a provider's HTTP status by provider key.
	providerStatus map[string]int
}

func strPtr(s string) *string { return &s }

func consentRequired() *policy.PdpResponse {
	return &policy.PdpResponse{
		AppAuthorized:           true,
		AppRequiresOwnerConsent: true,
		ConsentRequiredFields: []policy.ConsentRequiredField{
			{FieldName: "person.permanentAddress", SchemaID: "drp-v1", DisplayName: strPtr("Address")},
		},
	}
}

var goldenCases = []goldenCase{
	{
		name:    "single_provider_scalars",
		request: graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName address } }`},
	},
	{
		name:    "multi_provider_nested_object",
		request: graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName birthInfo { date place } } }`},
	},
	{
		name:    "array_field",
		request: graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName ownedVehicles { regNo make } } }`},
	},
	{
		name:    "all_providers",
		request: graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName address birthInfo { date } ownedVehicles { regNo } } }`},
	},
	{
		name: "variables",
		request: graphql.Request{
			Query:     `query GetPerson($nic: String!) { personInfo(nic: $nic) { fullName birthInfo { place } } }`,
			Variables: map[string]interface{}{"nic": "199012345678"},
		},
	},
	{
		name:    "pdp_skipped_when_not_configured",
		request: graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName } }`},
		noPDP:   true,
	},
	{
		name:    "pdp_denied",
		request: graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName address } }`},
		pdpResponse: &policy.PdpResponse{
			AppAuthorized:      false,
			UnauthorizedFields: []policy.ConsentRequiredField{{FieldName: "person.permanentAddress", SchemaID: "drp-v1"}},
		},
	},
	{
		name:    "pdp_access_expired",
		request: graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName } }`},
		pdpResponse: &policy.PdpResponse{
			AppAuthorized:    true,
			AppAccessExpired: true,
			ExpiredFields:    []policy.ConsentRequiredField{{FieldName: "person.fullName", SchemaID: "drp-v1"}},
		},
	},
	{
		name:      "pdp_error",
		request:   graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName } }`},
		pdpStatus: http.StatusInternalServerError,
	},
	{
		name:        "consent_approved",
		request:     graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName address } }`},
		pdpResponse: consentRequired(),
		ceResponse:  &consent.ConsentResponseInternalView{ConsentID: "consent-1", Status: consent.StatusApproved},
	},
	{
		name:        "consent_pending",
		request:     graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName address } }`},
		pdpResponse: consentRequired(),
		ceResponse: &consent.ConsentResponseInternalView{
			ConsentID:        "consent-2",
			Status:           consent.StatusPending,
			ConsentPortalURL: strPtr("https://consent.example/consents/consent-2"),
		},
	},
	{
		// A consent the owner revoked must not be served, even though it was approved before.
		name:        "consent_revoked",
		request:     graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName address } }`},
		pdpResponse: consentRequired(),
		// The consent engine sends a portal URL only for pending consents.
		ceResponse: &consent.ConsentResponseInternalView{ConsentID: "consent-3", Status: consent.StatusRevoked},
	},
	{
		name:    "missing_data_owner_id",
		request: graphql.Request{Query: `query { personInfo { fullName } }`},
	},
	{
		// Characterizes current behaviour: aliases are not honoured and only the
		// first nic is bound, so conflicting values are not detected.
		name:    "conflicting_data_owner_ids_via_aliases",
		request: graphql.Request{Query: `query { a: personInfo(nic: "111") { fullName } b: personInfo(nic: "222") { address } }`},
	},
	{
		name:    "mutation_rejected",
		request: graphql.Request{Query: `mutation { personInfo(nic: "199012345678") { fullName } }`},
	},
	{
		// Characterizes current behaviour: the parse error is ignored and the
		// pipeline panics on the nil document.
		name:    "invalid_query_syntax",
		request: graphql.Request{Query: `query { personInfo(nic: "199012345678" { fullName } }`},
	},
	{
		name:           "provider_failure",
		request:        graphql.Request{Query: `query { personInfo(nic: "199012345678") { fullName birthInfo { date } } }`},
		providerStatus: map[string]int{"rgd": http.StatusInternalServerError},
	},
}

func TestFederateQueryGolden(t *testing.T) {
	logger.Init()
	middleware.InitializeAuditConfig("SERVICE", "orchestration-engine")

	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			env := newGoldenEnv(t, tc)
			f := newGoldenFederator(t, env)

			got := env.run(t, f, tc.request)

			path := filepath.Join("testdata", "golden", tc.name+".json")
			if *update {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, got, 0o644))
				return
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "missing golden file; run with -update to create it")
			require.Equal(t, string(want), string(got))
		})
	}
}

// newGoldenFederator is the only place the golden suite constructs a Federator.
func newGoldenFederator(t *testing.T, env *goldenEnv) *federator.Federator {
	t.Helper()

	f, err := federator.Initialize(t.Context(), env.config, provider.NewProviderHandler(nil), nil)
	require.NoError(t, err)
	return f
}

type providerCall struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type auditEvent struct {
	EventType  string `json:"eventType"`
	Status     string `json:"status"`
	ActorType  string `json:"actorType"`
	ActorID    string `json:"actorId"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId,omitempty"`
}

type transcript struct {
	Panicked         bool                           `json:"panicked,omitempty"`
	Response         interface{}                    `json:"response"`
	PolicyRequests   []policy.PdpRequest            `json:"policyRequests"`
	ConsentRequests  []consent.CreateConsentRequest `json:"consentRequests"`
	ProviderRequests map[string][]providerCall      `json:"providerRequests"`
	AuditEvents      []auditEvent                   `json:"auditEvents"`
}

type goldenEnv struct {
	config *configs.Config
	audit  *recordingAuditor

	mu         sync.Mutex
	transcript transcript
}

func newGoldenEnv(t *testing.T, tc goldenCase) *goldenEnv {
	t.Helper()

	env := &goldenEnv{
		audit: &recordingAuditor{},
		transcript: transcript{
			PolicyRequests:   []policy.PdpRequest{},
			ConsentRequests:  []consent.CreateConsentRequest{},
			ProviderRequests: map[string][]providerCall{},
		},
	}

	auditpkg.ResetGlobalAuditMiddleware()
	auditpkg.InitializeGlobalAudit(env.audit)
	t.Cleanup(auditpkg.ResetGlobalAuditMiddleware)

	schema := goldenSchemaSDL
	env.config = &configs.Config{
		TrustUpstream:         true,
		FoundationalIdArgName: "nic",
		Schema:                &schema,
		ArgMapping:            goldenArgMappings,
		CeConfig:              configs.CeConfig{ClientURL: env.consentServer(t, tc).URL},
	}
	if !tc.noPDP {
		env.config.PdpConfig = configs.PdpConfig{ClientURL: env.pdpServer(t, tc).URL}
	}
	for _, p := range goldenProviders {
		env.config.Providers = append(env.config.Providers, &configs.ProviderConfig{
			ProviderKey: p.key,
			SchemaID:    p.schemaID,
			ProviderURL: env.providerServer(t, p, tc.providerStatus[p.key]).URL,
		})
	}
	return env
}

func (e *goldenEnv) pdpServer(t *testing.T, tc goldenCase) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req policy.PdpRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode PDP request: %v", err)
		}
		e.mu.Lock()
		e.transcript.PolicyRequests = append(e.transcript.PolicyRequests, req)
		e.mu.Unlock()

		if tc.pdpStatus != 0 && tc.pdpStatus != http.StatusOK {
			http.Error(w, "pdp unavailable", tc.pdpStatus)
			return
		}
		resp := tc.pdpResponse
		if resp == nil {
			resp = &policy.PdpResponse{AppAuthorized: true}
		}
		writeJSON(t, w, http.StatusOK, resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (e *goldenEnv) consentServer(t *testing.T, tc goldenCase) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req consent.CreateConsentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode consent request: %v", err)
		}
		e.mu.Lock()
		e.transcript.ConsentRequests = append(e.transcript.ConsentRequests, req)
		e.mu.Unlock()

		resp := tc.ceResponse
		if resp == nil {
			resp = &consent.ConsentResponseInternalView{ConsentID: "consent-default", Status: consent.StatusApproved}
		}
		writeJSON(t, w, http.StatusCreated, resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (e *goldenEnv) providerServer(t *testing.T, p goldenProvider, status int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req graphql.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode %s provider request: %v", p.key, err)
		}
		e.mu.Lock()
		e.transcript.ProviderRequests[p.key] = append(e.transcript.ProviderRequests[p.key], providerCall{
			Query:     canonicalQuery(req.Query),
			Variables: req.Variables,
		})
		e.mu.Unlock()

		if status != 0 && status != http.StatusOK {
			http.Error(w, "provider unavailable", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data": ` + p.data + `}`))
		if err != nil {
			t.Errorf("write %s provider response: %v", p.key, err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (e *goldenEnv) run(t *testing.T, f *federator.Federator, req graphql.Request) []byte {
	t.Helper()

	consumer := &auth.ConsumerAssertion{ApplicationID: "app-golden"}

	var resp graphql.Response
	func() {
		defer func() {
			if r := recover(); r != nil {
				e.transcript.Panicked = true
			}
		}()
		resp = f.FederateQuery(context.Background(), req, consumer)
	}()

	// Round-trip through JSON so the transcript reflects the wire format.
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &e.transcript.Response))

	for i := range e.transcript.PolicyRequests {
		fields := e.transcript.PolicyRequests[i].RequiredFields
		sort.Slice(fields, func(a, b int) bool {
			if fields[a].SchemaID != fields[b].SchemaID {
				return fields[a].SchemaID < fields[b].SchemaID
			}
			return fields[a].FieldName < fields[b].FieldName
		})
	}
	e.transcript.AuditEvents = e.audit.sortedEvents()

	out, err := json.MarshalIndent(e.transcript, "", "  ")
	require.NoError(t, err)
	return append(out, '\n')
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("write response: %v", err)
	}
}

// canonicalQuery reduces a GraphQL document to space-separated tokens so that
// formatting differences between printers (indentation, commas, spacing) don't
// affect the golden files.
func canonicalQuery(q string) string {
	var tokens []string
	for i := 0; i < len(q); {
		c := q[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',':
			i++
		case c == '#':
			for i < len(q) && q[i] != '\n' {
				i++
			}
		case c == '"':
			j := i + 1
			for j < len(q) && q[j] != '"' {
				if q[j] == '\\' {
					j++
				}
				j++
			}
			tokens = append(tokens, q[i:min(j+1, len(q))])
			i = j + 1
		case c == '.':
			j := i
			for j < len(q) && q[j] == '.' {
				j++
			}
			tokens = append(tokens, q[i:j])
			i = j
		case c == '-' || (c >= '0' && c <= '9'):
			j := i + 1
			for j < len(q) && strings.IndexByte("0123456789.eE+-", q[j]) >= 0 {
				j++
			}
			tokens = append(tokens, q[i:j])
			i = j
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			j := i + 1
			for j < len(q) && (q[j] == '_' || (q[j] >= 'a' && q[j] <= 'z') || (q[j] >= 'A' && q[j] <= 'Z') || (q[j] >= '0' && q[j] <= '9')) {
				j++
			}
			tokens = append(tokens, q[i:j])
			i = j
		default:
			tokens = append(tokens, string(c))
			i++
		}
	}
	return strings.Join(tokens, " ")
}

// recordingAuditor captures audit events emitted through the global argus middleware.
type recordingAuditor struct {
	mu     sync.Mutex
	events []auditEvent
}

// auditStageOrder gives provider fetches, which run concurrently, a stable
// position after the sequential pipeline stages.
var auditStageOrder = map[string]int{
	"DATA_REQUEST":   0,
	"POLICY_CHECK":   1,
	"CONSENT_CHECK":  2,
	"PROVIDER_FETCH": 3,
}

func (a *recordingAuditor) sortedEvents() []auditEvent {
	a.mu.Lock()
	defer a.mu.Unlock()

	events := append([]auditEvent{}, a.events...)
	sort.SliceStable(events, func(i, j int) bool {
		si, sj := auditStageOrder[events[i].EventType], auditStageOrder[events[j].EventType]
		if si != sj {
			return si < sj
		}
		return events[i].TargetID < events[j].TargetID
	})
	return events
}

func (a *recordingAuditor) LogEvent(_ context.Context, event *auditpkg.AuditLogRequest) bool {
	rec := auditEvent{
		EventType:  event.EventType,
		Status:     event.Status,
		ActorType:  event.ActorType,
		ActorID:    event.ActorID,
		TargetType: event.TargetType,
	}
	if event.TargetID != nil {
		rec.TargetID = *event.TargetID
	}
	a.mu.Lock()
	a.events = append(a.events, rec)
	a.mu.Unlock()
	return true
}

func (a *recordingAuditor) SignEvent(context.Context, *auditpkg.AuditLogRequest) error { return nil }

func (a *recordingAuditor) SignMessageBytes(context.Context, []byte) (string, error) { return "", nil }

func (a *recordingAuditor) LogSignedEvent(context.Context, *auditpkg.AuditLogRequest) {}

func (a *recordingAuditor) VerifyIntegrity(*auditpkg.AuditLogRequest, crypto.PublicKey) (bool, error) {
	return true, nil
}

func (a *recordingAuditor) IsEnabled() bool { return true }

func (a *recordingAuditor) Close(context.Context) error { return nil }
