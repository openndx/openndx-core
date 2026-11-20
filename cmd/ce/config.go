package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"slices"
	"time"

	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/database"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/logging"
)

// defaultConfigPath is used when neither -config nor CONFIG_PATH is set.
const defaultConfigPath = "config.yaml"

// Config is the CE's configuration, loaded from a single YAML file.
type Config struct {
	Server ServerConfig `yaml:"server"`
	// Environment names the deployment (e.g. local, staging, production). Log
	// lines carry it when it is set.
	Environment string         `yaml:"environment"`
	Logging     logging.Config `yaml:"logging"`
	// ConsentPortalURL is where data owners decide on consents. Consent links
	// are <url>?consentId=<uuid>, and its origin is always an allowed CORS origin.
	ConsentPortalURL string         `yaml:"consentPortalURL"`
	CORS             cors.Config    `yaml:"cors"`
	IDP              IDPConfig      `yaml:"idp"`
	Database         DatabaseConfig `yaml:"database"`
}

// ServerConfig holds the HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
	// Timeout bounds reading a request and writing its response.
	Timeout time.Duration `yaml:"timeout"`
}

// IDPConfig holds the configuration for validating consent-portal tokens.
type IDPConfig struct {
	// Config validates each token's signature, issuer, audience and expiry.
	authn.Config `yaml:",inline"`
	// SubjectClaim is the token claim carrying the data owner's canonical UID
	// (matched against the consent record's owner_id).
	SubjectClaim string `yaml:"subjectClaim"`
}

// DatabaseConfig is the shared database configuration plus the CE's own settings.
type DatabaseConfig struct {
	database.Config `yaml:",inline"`
	// IdleTimeout closes pooled connections that stay idle this long.
	IdleTimeout time.Duration `yaml:"idleTimeout"`
}

// defaultConfig returns the values used for any key the config file leaves out.
func defaultConfig() Config {
	return Config{
		Server:  ServerConfig{Port: 8081, Timeout: 10 * time.Second},
		Logging: logging.DefaultConfig(),
		CORS: cors.Config{
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowedHeaders: []string{
				"Origin", "Content-Type", "Accept", "Authorization",
				"X-Requested-With", "X-CSRF-Token", "X-Request-ID",
			},
			AllowCredentials: true,
			MaxAge:           86400,
		},
		IDP: IDPConfig{SubjectClaim: "sub"},
		Database: DatabaseConfig{
			Config: database.Config{
				Driver: database.Postgres,
				Postgres: &database.PostgresConfig{
					Port:    5432,
					Name:    "consent_engine",
					SSLMode: "require",
					Pool:    database.PoolConfig{MaxOpenConns: 25, MaxIdleConns: 5, MaxConnLifetimeSeconds: 3600},
				},
			},
			IdleTimeout: 30 * time.Minute,
		},
	}
}

// loadConfig reads the config file named by -config, CONFIG_PATH or
// config.yaml, in that order, on top of the defaults and validates the result.
func loadConfig(args []string) (*Config, error) {
	flags := flag.NewFlagSet("consent-engine", flag.ContinueOnError)
	path := flags.String("config", configPathFromEnv(), "Path to the YAML config file")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	if err := configyaml.LoadAndExpand(*path, &cfg); err != nil {
		return nil, err
	}
	if origin := portalOrigin(cfg.ConsentPortalURL); origin != "" && !slices.Contains(cfg.CORS.AllowedOrigins, origin) {
		cfg.CORS.AllowedOrigins = append(cfg.CORS.AllowedOrigins, origin)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config file %s: %w", *path, err)
	}
	return &cfg, nil
}

// portalOrigin returns the scheme and host of the consent portal URL, the form
// browsers send in the Origin header, or "" when the URL has neither.
func portalOrigin(portalURL string) string {
	portal, err := url.Parse(portalURL)
	if err != nil || portal.Scheme == "" || portal.Host == "" {
		return ""
	}
	return portal.Scheme + "://" + portal.Host
}

// configPathFromEnv returns CONFIG_PATH, or config.yaml when it is unset.
func configPathFromEnv() string {
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}
	return defaultConfigPath
}

// Validate checks the configuration before startup.
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if c.Server.Timeout <= 0 {
		return fmt.Errorf("server.timeout must be positive")
	}
	if err := c.Logging.Validate(); err != nil {
		return err
	}
	portal, err := url.Parse(c.ConsentPortalURL)
	if err != nil || portal.Scheme == "" || portal.Host == "" {
		return fmt.Errorf("consentPortalURL must include a scheme and host")
	}
	if err := c.CORS.Validate(); err != nil {
		return fmt.Errorf("invalid cors configuration: %w", err)
	}
	if err := c.IDP.Validate(); err != nil {
		return fmt.Errorf("invalid idp configuration: %w", err)
	}
	if c.IDP.SubjectClaim == "" {
		return fmt.Errorf("idp.subjectClaim must not be empty")
	}
	if c.Database.Driver != database.Postgres {
		return fmt.Errorf("database.driver must be postgres")
	}
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}
	pg := c.Database.Postgres
	if pg.Port < 1 || pg.Port > 65535 {
		return fmt.Errorf("database.postgres.port must be between 1 and 65535")
	}
	if pg.SSLMode == "" {
		return fmt.Errorf("database.postgres.sslMode is required")
	}
	if pg.Pool.MaxOpenConns <= 0 || pg.Pool.MaxIdleConns <= 0 || pg.Pool.MaxIdleConns > pg.Pool.MaxOpenConns || pg.Pool.MaxConnLifetimeSeconds <= 0 {
		return fmt.Errorf("invalid database.postgres.pool: require positive connection limits and lifetime, with maxIdleConns no greater than maxOpenConns")
	}
	if c.Database.IdleTimeout <= 0 {
		return fmt.Errorf("database.idleTimeout must be positive")
	}
	return nil
}
