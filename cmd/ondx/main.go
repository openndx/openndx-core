// Command ondx is a CLI for OpenNDX management operations against the Portal Backend.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/openndx/openndx-core/internal/cli/auth"
	"github.com/openndx/openndx-core/internal/cli/pbclient"
	"github.com/openndx/openndx-core/internal/cli/profile"
	"github.com/openndx/openndx-core/internal/pb/models"
)

// newHTTPClient builds an HTTP client for talking to the identity provider or
// Portal Backend. insecureSkipVerify exists for local dev against ThunderID's
// self-signed dev certificate (its own compose healthcheck uses `curl -k` for
// the same reason) — never pass true against a real deployment.
func newHTTPClient(insecureSkipVerify bool) *http.Client {
	client := &http.Client{Timeout: 15 * time.Second}
	if insecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in local-dev flag only
		}
	}
	return client
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "login":
		err = runLogin(ctx, os.Args[2:])
	case "profile":
		err = runProfile(os.Args[2:])
	case "policy":
		err = runPolicy(ctx, os.Args[2:])
	case "applications":
		err = runApplications(ctx, os.Args[2:])
	case "members":
		err = runMembers(ctx, os.Args[2:])
	case "schemas":
		err = runSchemas(ctx, os.Args[2:])
	case "version", "--version":
		err = runVersion(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "ondx: unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "ondx: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `ondx - OpenNDX management CLI

Usage:
  ondx login  [flags]               Log in via your browser and cache a token
  ondx profile list                 List configured profiles
  ondx profile use <name>           Switch the active profile
  ondx profile set <name> [flags]   Create or update a named profile
  ondx members create [flags]       Register a new member
  ondx schemas create [flags]       Register a new schema and its grantable fields
  ondx applications create [flags]  Register a new application
  ondx applications list [flags]    List applications
  ondx applications get [flags]     Show an application's current details and policy
  ondx policy update [flags]        Update an existing application's policy
  ondx version                      Print the ondx version and build info

Every command above accepts --profile <name> (env NDX_PROFILE) to use a
profile other than the current one for that invocation - see
'ondx profile list' and 'ondx profile set -h'. A "local" profile matching this
repo's docker-compose local-dev stack is built in, so 'ondx login' works with
no flags at all until you configure other profiles.

Run 'ondx <command> -h' for flags on a specific command.
`)
}

// stringSlice implements flag.Value to collect a repeated flag into a slice.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// keyValueMap implements flag.Value to collect repeated "key=value" flags into a map.
type keyValueMap map[string]string

func (m keyValueMap) String() string { return fmt.Sprintf("%v", map[string]string(m)) }
func (m keyValueMap) Set(v string) error {
	key, value, found := strings.Cut(v, "=")
	if !found {
		return fmt.Errorf("expected key=value, got %q", v)
	}
	m[key] = value
	return nil
}

func defaultCredentialsPathOrExit(profileName string) string {
	path, err := auth.DefaultCredentialsPath(profileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ondx: %v\n", err)
		os.Exit(1)
	}
	return path
}

// firstNonEmpty returns the first non-empty string, in priority order.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// extractFlagValue scans args for a flag's value without fully parsing the
// set. It exists to resolve --profile before a command's real flag.FlagSet
// is built, since that set's other flags need the resolved profile's values
// as their defaults, and flag defaults must be fixed before flag.Parse runs.
func extractFlagValue(args []string, name string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		for _, prefix := range []string{"--" + name + "=", "-" + name + "="} {
			if strings.HasPrefix(a, prefix) {
				return strings.TrimPrefix(a, prefix)
			}
		}
		if (a == "--"+name || a == "-"+name) && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// resolveActiveProfile determines which profile a command should use - an
// explicit --profile flag or NDX_PROFILE if given, otherwise the config
// file's current profile - and returns its values to seed the command's
// other flag defaults.
func resolveActiveProfile(args []string) (name string, active profile.Profile, err error) {
	name = firstNonEmpty(extractFlagValue(args, "profile"), os.Getenv("NDX_PROFILE"))

	path, err := profile.DefaultConfigPath()
	if err != nil {
		return "", profile.Profile{}, err
	}
	cfg, err := profile.Load(path)
	if err != nil {
		return "", profile.Profile{}, err
	}
	if name == "" {
		name = cfg.CurrentProfile
	}
	active, err = cfg.Get(name)
	if err != nil {
		return "", profile.Profile{}, err
	}
	return name, active, nil
}

func runProfile(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("expected a subcommand: 'list', 'use', or 'set' (usage: ondx profile list|use|set [flags])")
	}
	switch args[0] {
	case "list":
		return runProfileList()
	case "use":
		return runProfileUse(args[1:])
	case "set":
		return runProfileSet(args[1:])
	default:
		return fmt.Errorf("unknown profile subcommand %q (expected 'list', 'use', or 'set')", args[0])
	}
}

func runProfileList() error {
	path, err := profile.DefaultConfigPath()
	if err != nil {
		return err
	}
	cfg, err := profile.Load(path)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		marker := " "
		if name == cfg.CurrentProfile {
			marker = "*"
		}
		fmt.Printf("%s %s\n", marker, name)
	}
	return nil
}

func runProfileUse(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("expected a profile name (usage: ondx profile use <name>)")
	}
	name := args[0]

	path, err := profile.DefaultConfigPath()
	if err != nil {
		return err
	}
	cfg, err := profile.Load(path)
	if err != nil {
		return err
	}
	if _, err := cfg.Get(name); err != nil {
		return err
	}

	cfg.CurrentProfile = name
	if err := profile.Save(path, cfg); err != nil {
		return err
	}
	fmt.Printf("Now using profile %q.\n", name)
	return nil
}

func runProfileSet(args []string) error {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("expected a profile name (usage: ondx profile set <name> [flags])")
	}
	name := args[0]

	fs := flag.NewFlagSet("profile set", flag.ExitOnError)
	issuer := fs.String("issuer", "", "Identity provider issuer/base URL")
	authURL := fs.String("auth-url", "", "Identity provider authorization endpoint (overrides issuer-based discovery)")
	tokenURL := fs.String("token-url", "", "Identity provider token endpoint (overrides issuer-based discovery)")
	clientID := fs.String("client-id", "", "OAuth2 public client ID")
	scopes := fs.String("scopes", "", "Space-separated OAuth2 scopes to request")
	callbackPort := fs.Int("callback-port", 0, "Fixed local port for the OAuth2 redirect callback")
	pbURL := fs.String("pb-url", "", "Portal Backend base URL")
	insecure := fs.Bool("insecure", false, "Skip TLS certificate verification")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage of profile set <name>:")
		fs.PrintDefaults()
		fmt.Fprint(fs.Output(), "\nOnly flags actually passed are changed - existing values for any flag\n"+
			"you omit are left as they are.\n")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	path, err := profile.DefaultConfigPath()
	if err != nil {
		return err
	}
	cfg, err := profile.Load(path)
	if err != nil {
		return err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]profile.Profile{}
	}

	p := cfg.Profiles[name]
	changed := false
	fs.Visit(func(f *flag.Flag) {
		changed = true
		switch f.Name {
		case "issuer":
			p.Issuer = *issuer
		case "auth-url":
			p.AuthURL = *authURL
		case "token-url":
			p.TokenURL = *tokenURL
		case "client-id":
			p.ClientID = *clientID
		case "scopes":
			p.Scopes = *scopes
		case "callback-port":
			p.CallbackPort = *callbackPort
		case "pb-url":
			p.PBURL = *pbURL
		case "insecure":
			p.Insecure = *insecure
		}
	})
	if !changed {
		return fmt.Errorf("at least one flag is required to set a value, e.g. --issuer https://idp.example.com")
	}

	cfg.Profiles[name] = p
	if err := profile.Save(path, cfg); err != nil {
		return err
	}
	fmt.Printf("Profile %q saved.\n", name)
	return nil
}

func runLogin(ctx context.Context, args []string) error {
	profileName, active, err := resolveActiveProfile(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("login", flag.ExitOnError)
	fs.String("profile", profileName, "Named profile to use for defaults (env NDX_PROFILE; see 'ondx profile list')")
	issuer := fs.String("issuer", firstNonEmpty(os.Getenv("NDX_ISSUER"), active.Issuer), "Identity provider issuer/base URL (env NDX_ISSUER) - auth-url/token-url are discovered from {issuer}/.well-known/openid-configuration when set")
	authURL := fs.String("auth-url", firstNonEmpty(os.Getenv("NDX_AUTH_URL"), active.AuthURL), "Identity provider authorization endpoint (env NDX_AUTH_URL). Overrides discovery from --issuer if both are set")
	tokenURL := fs.String("token-url", firstNonEmpty(os.Getenv("NDX_TOKEN_URL"), active.TokenURL), "Identity provider token endpoint (env NDX_TOKEN_URL). Overrides discovery from --issuer if both are set")
	clientID := fs.String("client-id", firstNonEmpty(os.Getenv("NDX_CLIENT_ID"), active.ClientID), "OAuth2 public client ID registered with the identity provider (env NDX_CLIENT_ID)")
	scopes := fs.String("scopes", firstNonEmpty(os.Getenv("NDX_SCOPES"), active.Scopes), "Space-separated OAuth2 scopes to request (env NDX_SCOPES)")
	noBrowser := fs.Bool("no-browser", false, "Print the login URL instead of opening a browser automatically")
	credentialsPath := fs.String("credentials-path", "", "Path to store the cached token (default ~/.openndx/credentials.json, or credentials-<profile>.json for a non-default profile)")
	callbackPort := fs.Int("callback-port", active.CallbackPort, "Fixed local port for the OAuth2 redirect callback (0 = OS-assigned random port). Required if the identity provider's redirect URI allow-list needs an exact match rather than a wildcard port")
	insecure := fs.Bool("insecure", active.Insecure, "Skip TLS certificate verification (local dev only, e.g. against ThunderID's self-signed cert)")
	extraParams := keyValueMap{}
	fs.Var(extraParams, "extra", "Extra authorization query param as key=value (repeatable, e.g. resource=http://api.openndx.local)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *clientID == "" {
		return fmt.Errorf("--client-id is required (or set NDX_CLIENT_ID, or configure it in a profile)")
	}

	if *clientID == profile.ThunderIDCLIClientID && *callbackPort != profile.ThunderIDCallbackPort {
		return fmt.Errorf("--callback-port must be %d for client %q: ThunderID only accepts the exact redirect URI registered for it in thunderid/bootstrap/application.yaml (http://127.0.0.1:%d/callback)", profile.ThunderIDCallbackPort, profile.ThunderIDCLIClientID, profile.ThunderIDCallbackPort)
	}

	httpClient := newHTTPClient(*insecure)

	if *issuer != "" && (*authURL == "" || *tokenURL == "") {
		discoveredAuthURL, discoveredTokenURL, err := auth.DiscoverEndpoints(ctx, httpClient, *issuer)
		if err != nil {
			return fmt.Errorf("OIDC discovery failed for issuer %s: %w (pass --auth-url and --token-url explicitly if this identity provider doesn't support discovery at that path)", *issuer, err)
		}
		if *authURL == "" {
			*authURL = discoveredAuthURL
		}
		if *tokenURL == "" {
			*tokenURL = discoveredTokenURL
		}
	}

	if *authURL == "" || *tokenURL == "" {
		return fmt.Errorf("--auth-url and --token-url are required unless --issuer supports OIDC discovery (or set NDX_AUTH_URL/NDX_TOKEN_URL/NDX_ISSUER, or configure them in a profile)")
	}

	path := *credentialsPath
	if path == "" {
		path = defaultCredentialsPathOrExit(profileName)
	}

	token, err := auth.Login(ctx, auth.LoginOptions{
		AuthURL:      *authURL,
		TokenURL:     *tokenURL,
		ClientID:     *clientID,
		Scopes:       *scopes,
		ExtraParams:  extraParams,
		OpenBrowser:  !*noBrowser,
		CallbackPort: *callbackPort,
		HTTPClient:   httpClient,
	})
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	if err := auth.SaveToken(path, token); err != nil {
		return fmt.Errorf("login succeeded but failed to save credentials: %w", err)
	}

	fmt.Printf("Logged in (profile %q). Credentials cached at %s\n", profileName, path)
	return nil
}

func runPolicy(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("expected a subcommand: 'update' (usage: ondx policy update [flags])")
	}
	switch args[0] {
	case "update":
		return runPolicyUpdate(ctx, args[1:])
	default:
		return fmt.Errorf("unknown policy subcommand %q (expected 'update')", args[0])
	}
}

func runPolicyUpdate(ctx context.Context, args []string) error {
	profileName, active, err := resolveActiveProfile(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("policy update", flag.ExitOnError)
	fs.String("profile", profileName, "Named profile to use for defaults (env NDX_PROFILE; see 'ondx profile list')")
	appID := fs.String("app-id", "", "Application ID to update (required)")
	grantDuration := fs.String("grant-duration", "", "Grant duration for the granted fields, e.g. 30d or 365d (default: server default)")
	pbURL := fs.String("pb-url", firstNonEmpty(os.Getenv("NDX_PB_URL"), active.PBURL), "Portal Backend base URL (env NDX_PB_URL)")
	credentialsPath := fs.String("credentials-path", "", "Path to the cached token (default ~/.openndx/credentials.json, or credentials-<profile>.json for a non-default profile)")
	insecure := fs.Bool("insecure", active.Insecure, "Skip TLS certificate verification (local dev only, e.g. against ThunderID's self-signed cert)")
	var fields stringSlice
	fs.Var(&fields, "field", "Field to grant, as schemaId:fieldName (repeatable, required)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *appID == "" {
		return fmt.Errorf("--app-id is required")
	}
	if len(fields) == 0 {
		return fmt.Errorf("at least one --field schemaId:fieldName is required")
	}
	if *pbURL == "" {
		return fmt.Errorf("--pb-url is required (or set NDX_PB_URL, or configure it in a profile)")
	}

	selectedFields := make([]models.SelectedFieldRecord, 0, len(fields))
	for _, f := range fields {
		schemaID, fieldName, found := strings.Cut(f, ":")
		if !found || schemaID == "" || fieldName == "" {
			return fmt.Errorf("invalid --field %q: expected schemaId:fieldName", f)
		}
		selectedFields = append(selectedFields, models.SelectedFieldRecord{
			SchemaID:  schemaID,
			FieldName: fieldName,
		})
	}

	req := &models.UpdateApplicationPolicyRequest{SelectedFields: selectedFields}
	if *grantDuration != "" {
		gd := models.GrantDurationType(*grantDuration)
		req.GrantDuration = &gd
	}

	path := *credentialsPath
	if path == "" {
		path = defaultCredentialsPathOrExit(profileName)
	}

	httpClient := newHTTPClient(*insecure)

	token, err := auth.EnsureFreshToken(ctx, path, httpClient)
	if err != nil {
		return err
	}

	client := pbclient.NewClient(*pbURL, token.AccessToken)
	client.HTTPClient = httpClient
	app, err := client.UpdateApplicationPolicy(ctx, *appID, req)
	if err != nil {
		return fmt.Errorf("failed to update application policy: %w", err)
	}

	fmt.Printf("Updated policy for application %s (%s):\n", app.ApplicationID, app.ApplicationName)
	for _, f := range app.SelectedFields {
		fmt.Printf("  - %s (schema %s)\n", f.FieldName, f.SchemaID)
	}
	return nil
}

func runMembers(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("expected a subcommand: 'create' (usage: ondx members create [flags])")
	}
	switch args[0] {
	case "create":
		return runMembersCreate(ctx, args[1:])
	default:
		return fmt.Errorf("unknown members subcommand %q (expected 'create')", args[0])
	}
}

func runMembersCreate(ctx context.Context, args []string) error {
	profileName, active, err := resolveActiveProfile(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("members create", flag.ExitOnError)
	fs.String("profile", profileName, "Named profile to use for defaults (env NDX_PROFILE; see 'ondx profile list')")
	name := fs.String("name", "", "Member name (required)")
	email := fs.String("email", "", "Member email (required)")
	phone := fs.String("phone", "", "Member phone number (required)")
	idpUserID := fs.String("idp-user-id", "", "Pre-provisioned IDP user ID (see note below)")
	pbURL := fs.String("pb-url", firstNonEmpty(os.Getenv("NDX_PB_URL"), active.PBURL), "Portal Backend base URL (env NDX_PB_URL)")
	credentialsPath := fs.String("credentials-path", "", "Path to the cached token (default ~/.openndx/credentials.json, or credentials-<profile>.json for a non-default profile)")
	insecure := fs.Bool("insecure", active.Insecure, "Skip TLS certificate verification (local dev only, e.g. against ThunderID's self-signed cert)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage of members create:")
		fs.PrintDefaults()
		fmt.Fprint(fs.Output(), "\nIf -idp-user-id is omitted, Portal Backend provisions the user in the IDP\n"+
			"itself (create account + assign to the member group) - this only works\n"+
			"against an Asgardeo (WSO2) IDP today, not ThunderID. Against ThunderID,\n"+
			"create the user manually via its console first (and assign whatever\n"+
			"group/role that person needs), then pass their user id here.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *email == "" {
		return fmt.Errorf("--email is required")
	}
	if *phone == "" {
		return fmt.Errorf("--phone is required")
	}
	if *pbURL == "" {
		return fmt.Errorf("--pb-url is required (or set NDX_PB_URL, or configure it in a profile)")
	}

	req := &models.CreateMemberRequest{
		Name:        *name,
		Email:       *email,
		PhoneNumber: *phone,
	}
	if *idpUserID != "" {
		req.IdpUserID = idpUserID
	}

	path := *credentialsPath
	if path == "" {
		path = defaultCredentialsPathOrExit(profileName)
	}

	httpClient := newHTTPClient(*insecure)

	token, err := auth.EnsureFreshToken(ctx, path, httpClient)
	if err != nil {
		return err
	}

	client := pbclient.NewClient(*pbURL, token.AccessToken)
	client.HTTPClient = httpClient
	member, err := client.CreateMember(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create member: %w", err)
	}

	fmt.Printf("Created member %s (%s)\n", member.MemberID, member.Name)
	fmt.Printf("Email:       %s\n", member.Email)
	fmt.Printf("IdP User:    %s\n", member.IdpUserID)
	return nil
}

func runSchemas(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("expected a subcommand: 'create' (usage: ondx schemas create [flags])")
	}
	switch args[0] {
	case "create":
		return runSchemasCreate(ctx, args[1:])
	default:
		return fmt.Errorf("unknown schemas subcommand %q (expected 'create')", args[0])
	}
}

func runSchemasCreate(ctx context.Context, args []string) error {
	profileName, active, err := resolveActiveProfile(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("schemas create", flag.ExitOnError)
	fs.String("profile", profileName, "Named profile to use for defaults (env NDX_PROFILE; see 'ondx profile list')")
	name := fs.String("name", "", "Schema name (required)")
	description := fs.String("description", "", "Schema description")
	endpoint := fs.String("endpoint", "", "Provider GraphQL endpoint (required)")
	memberID := fs.String("member-id", "", "Owning member ID (required)")
	pbURL := fs.String("pb-url", firstNonEmpty(os.Getenv("NDX_PB_URL"), active.PBURL), "Portal Backend base URL (env NDX_PB_URL)")
	credentialsPath := fs.String("credentials-path", "", "Path to the cached token (default ~/.openndx/credentials.json, or credentials-<profile>.json for a non-default profile)")
	insecure := fs.Bool("insecure", active.Insecure, "Skip TLS certificate verification (local dev only, e.g. against ThunderID's self-signed cert)")
	var fields stringSlice
	fs.Var(&fields, "field", "Grantable field, as fieldName:accessControlType:source[:isOwner] (repeatable, required). accessControlType is 'public' or 'restricted'; source is 'primary' or 'fallback'; append ':isOwner' to mark this field as the record's owner-identifying field.")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage of schemas create:")
		fs.PrintDefaults()
		fmt.Fprint(fs.Output(), "\nEach -field declares one policy-metadata record directly, skipping GraphQL\n"+
			"SDL parsing entirely - the fieldName is used as-is (no typename. prefix).\n"+
			"Example: -field email:public:primary\n"+
			"By default, isOwner is false and owner is set to \"citizen\" (the only\n"+
			"owner value this system currently supports). Append \":isOwner\" to mark\n"+
			"a field as the owner-identifying field instead, e.g. -field nic:public:primary:isOwner\n"+
			"(isOwner true and owner unset are mutually exclusive - the PDP rejects\n"+
			"records that don't follow exactly one of these two shapes).\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *endpoint == "" {
		return fmt.Errorf("--endpoint is required")
	}
	if *memberID == "" {
		return fmt.Errorf("--member-id is required")
	}
	if len(fields) == 0 {
		return fmt.Errorf("at least one --field fieldName:accessControlType:source is required")
	}
	if *pbURL == "" {
		return fmt.Errorf("--pb-url is required (or set NDX_PB_URL, or configure it in a profile)")
	}

	records := make([]models.PolicyMetadataCreateRequestRecord, 0, len(fields))
	for _, f := range fields {
		parts := strings.Split(f, ":")
		if len(parts) < 3 || len(parts) > 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return fmt.Errorf("invalid --field %q: expected fieldName:accessControlType:source[:isOwner]", f)
		}
		isOwner := false
		if len(parts) == 4 {
			if parts[3] != "isOwner" {
				return fmt.Errorf("invalid --field %q: the optional 4th part must be exactly \"isOwner\"", f)
			}
			isOwner = true
		}

		record := models.PolicyMetadataCreateRequestRecord{
			FieldName:         parts[0],
			AccessControlType: models.AccessControlType(parts[1]),
			Source:            models.Source(parts[2]),
			IsOwner:           isOwner,
		}
		// The PDP requires owner to be set when isOwner is false, and unset
		// when isOwner is true - "citizen" is the only owner value this
		// system currently supports.
		if !isOwner {
			owner := models.OwnerCitizen
			record.Owner = &owner
		}
		records = append(records, record)
	}

	req := &models.CreateSchemaRequest{
		SchemaName: *name,
		Endpoint:   *endpoint,
		MemberID:   *memberID,
		Fields:     records,
	}
	if *description != "" {
		req.SchemaDescription = description
	}

	path := *credentialsPath
	if path == "" {
		path = defaultCredentialsPathOrExit(profileName)
	}

	httpClient := newHTTPClient(*insecure)

	token, err := auth.EnsureFreshToken(ctx, path, httpClient)
	if err != nil {
		return err
	}

	client := pbclient.NewClient(*pbURL, token.AccessToken)
	client.HTTPClient = httpClient
	schema, err := client.CreateSchema(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	fmt.Printf("Created schema %s (%s)\n", schema.SchemaID, schema.SchemaName)
	fmt.Println("Fields:")
	for _, r := range records {
		fmt.Printf("  - %s:%s (%s)\n", schema.SchemaID, r.FieldName, r.AccessControlType)
	}
	return nil
}

func runApplications(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("expected a subcommand: 'create', 'get', or 'list' (usage: ondx applications create|get|list [flags])")
	}
	switch args[0] {
	case "create":
		return runApplicationsCreate(ctx, args[1:])
	case "get":
		return runApplicationsGet(ctx, args[1:])
	case "list":
		return runApplicationsList(ctx, args[1:])
	default:
		return fmt.Errorf("unknown applications subcommand %q (expected 'create', 'get', or 'list')", args[0])
	}
}

func runApplicationsCreate(ctx context.Context, args []string) error {
	profileName, active, err := resolveActiveProfile(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("applications create", flag.ExitOnError)
	fs.String("profile", profileName, "Named profile to use for defaults (env NDX_PROFILE; see 'ondx profile list')")
	name := fs.String("name", "", "Application name (required)")
	description := fs.String("description", "", "Application description")
	memberID := fs.String("member-id", "", "Owning member ID (required)")
	idpApplicationID := fs.String("idp-application-id", "", "Pre-provisioned IDP application ID (must be paired with --idp-client-id; see note below)")
	idpClientID := fs.String("idp-client-id", "", "Pre-provisioned IDP OAuth2 client ID (must be paired with --idp-application-id; see note below)")
	pbURL := fs.String("pb-url", firstNonEmpty(os.Getenv("NDX_PB_URL"), active.PBURL), "Portal Backend base URL (env NDX_PB_URL)")
	credentialsPath := fs.String("credentials-path", "", "Path to the cached token (default ~/.openndx/credentials.json, or credentials-<profile>.json for a non-default profile)")
	insecure := fs.Bool("insecure", active.Insecure, "Skip TLS certificate verification (local dev only, e.g. against ThunderID's self-signed cert)")
	var fields stringSlice
	fs.Var(&fields, "field", "Field to grant, as schemaId:fieldName (repeatable, required)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage of applications create:")
		fs.PrintDefaults()
		fmt.Fprint(fs.Output(), "\nIf -idp-application-id/-idp-client-id are both omitted, Portal Backend\n"+
			"provisions the OAuth2 client itself - this only works against an Asgardeo\n"+
			"(WSO2) IDP today, not ThunderID. Against ThunderID, onboard the OAuth2 client\n"+
			"manually via its console first, then pass its resource id and clientId here\n"+
			"together.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *memberID == "" {
		return fmt.Errorf("--member-id is required")
	}
	if len(fields) == 0 {
		return fmt.Errorf("at least one --field schemaId:fieldName is required")
	}
	if (*idpApplicationID == "") != (*idpClientID == "") {
		return fmt.Errorf("--idp-application-id and --idp-client-id must both be set, or both omitted")
	}
	if *pbURL == "" {
		return fmt.Errorf("--pb-url is required (or set NDX_PB_URL, or configure it in a profile)")
	}

	selectedFields := make([]models.SelectedFieldRecord, 0, len(fields))
	for _, f := range fields {
		schemaID, fieldName, found := strings.Cut(f, ":")
		if !found || schemaID == "" || fieldName == "" {
			return fmt.Errorf("invalid --field %q: expected schemaId:fieldName", f)
		}
		selectedFields = append(selectedFields, models.SelectedFieldRecord{
			SchemaID:  schemaID,
			FieldName: fieldName,
		})
	}

	req := &models.CreateApplicationRequest{
		ApplicationName: *name,
		SelectedFields:  selectedFields,
		MemberID:        *memberID,
	}
	if *description != "" {
		req.ApplicationDescription = description
	}
	if *idpApplicationID != "" {
		req.IdpApplicationID = idpApplicationID
		req.IdpClientID = idpClientID
	}

	path := *credentialsPath
	if path == "" {
		path = defaultCredentialsPathOrExit(profileName)
	}

	httpClient := newHTTPClient(*insecure)

	token, err := auth.EnsureFreshToken(ctx, path, httpClient)
	if err != nil {
		return err
	}

	client := pbclient.NewClient(*pbURL, token.AccessToken)
	client.HTTPClient = httpClient
	app, err := client.CreateApplication(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}

	fmt.Printf("Created application %s (%s)\n", app.ApplicationID, app.ApplicationName)
	if app.IdpClientID != nil {
		fmt.Printf("IdP Client:  %s\n", *app.IdpClientID)
	}
	fmt.Println("Selected fields (current policy):")
	for _, f := range app.SelectedFields {
		fmt.Printf("  - %s:%s\n", f.SchemaID, f.FieldName)
	}
	return nil
}

func runApplicationsList(ctx context.Context, args []string) error {
	profileName, active, err := resolveActiveProfile(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("applications list", flag.ExitOnError)
	fs.String("profile", profileName, "Named profile to use for defaults (env NDX_PROFILE; see 'ondx profile list')")
	memberID := fs.String("member-id", "", "Filter to a single member's applications (admins see all applications if omitted)")
	pbURL := fs.String("pb-url", firstNonEmpty(os.Getenv("NDX_PB_URL"), active.PBURL), "Portal Backend base URL (env NDX_PB_URL)")
	credentialsPath := fs.String("credentials-path", "", "Path to the cached token (default ~/.openndx/credentials.json, or credentials-<profile>.json for a non-default profile)")
	insecure := fs.Bool("insecure", active.Insecure, "Skip TLS certificate verification (local dev only, e.g. against ThunderID's self-signed cert)")
	jsonOutput := fs.Bool("json", false, "Print the raw JSON response instead of a formatted summary")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *pbURL == "" {
		return fmt.Errorf("--pb-url is required (or set NDX_PB_URL, or configure it in a profile)")
	}

	path := *credentialsPath
	if path == "" {
		path = defaultCredentialsPathOrExit(profileName)
	}

	httpClient := newHTTPClient(*insecure)

	token, err := auth.EnsureFreshToken(ctx, path, httpClient)
	if err != nil {
		return err
	}

	client := pbclient.NewClient(*pbURL, token.AccessToken)
	client.HTTPClient = httpClient

	var memberFilter *string
	if *memberID != "" {
		memberFilter = memberID
	}
	apps, err := client.ListApplications(ctx, memberFilter)
	if err != nil {
		return fmt.Errorf("failed to list applications: %w", err)
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(apps)
	}

	if len(apps.Items) == 0 {
		fmt.Println("No applications found.")
		return nil
	}
	fmt.Printf("%-40s %-30s %-40s %s\n", "APPLICATION ID", "NAME", "MEMBER ID", "FIELDS")
	for _, app := range apps.Items {
		fmt.Printf("%-40s %-30s %-40s %d\n", app.ApplicationID, app.ApplicationName, app.MemberID, len(app.SelectedFields))
	}
	return nil
}

func runApplicationsGet(ctx context.Context, args []string) error {
	profileName, active, err := resolveActiveProfile(args)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("applications get", flag.ExitOnError)
	fs.String("profile", profileName, "Named profile to use for defaults (env NDX_PROFILE; see 'ondx profile list')")
	appID := fs.String("app-id", "", "Application ID to fetch (required)")
	pbURL := fs.String("pb-url", firstNonEmpty(os.Getenv("NDX_PB_URL"), active.PBURL), "Portal Backend base URL (env NDX_PB_URL)")
	credentialsPath := fs.String("credentials-path", "", "Path to the cached token (default ~/.openndx/credentials.json, or credentials-<profile>.json for a non-default profile)")
	insecure := fs.Bool("insecure", active.Insecure, "Skip TLS certificate verification (local dev only, e.g. against ThunderID's self-signed cert)")
	jsonOutput := fs.Bool("json", false, "Print the raw JSON response instead of a formatted summary")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *appID == "" {
		return fmt.Errorf("--app-id is required")
	}
	if *pbURL == "" {
		return fmt.Errorf("--pb-url is required (or set NDX_PB_URL, or configure it in a profile)")
	}

	path := *credentialsPath
	if path == "" {
		path = defaultCredentialsPathOrExit(profileName)
	}

	httpClient := newHTTPClient(*insecure)

	token, err := auth.EnsureFreshToken(ctx, path, httpClient)
	if err != nil {
		return err
	}

	client := pbclient.NewClient(*pbURL, token.AccessToken)
	client.HTTPClient = httpClient
	app, err := client.GetApplication(ctx, *appID)
	if err != nil {
		return fmt.Errorf("failed to fetch application: %w", err)
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(app)
	}

	fmt.Printf("Application: %s (%s)\n", app.ApplicationName, app.ApplicationID)
	if app.ApplicationDescription != nil && *app.ApplicationDescription != "" {
		fmt.Printf("Description: %s\n", *app.ApplicationDescription)
	}
	fmt.Printf("Member ID:   %s\n", app.MemberID)
	fmt.Printf("Version:     %s\n", app.Version)
	if app.IdpClientID != nil {
		fmt.Printf("IdP Client:  %s\n", *app.IdpClientID)
	}
	fmt.Printf("Created:     %s\n", app.CreatedAt)
	fmt.Printf("Updated:     %s\n", app.UpdatedAt)
	fmt.Println("Selected fields (current policy):")
	if len(app.SelectedFields) == 0 {
		fmt.Println("  (none)")
	}
	for _, f := range app.SelectedFields {
		fmt.Printf("  - %s:%s\n", f.SchemaID, f.FieldName)
	}
	return nil
}
