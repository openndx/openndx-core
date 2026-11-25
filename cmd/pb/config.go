// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/core/shared/validation"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/logging"
	"github.com/openndx/openndx-core/internal/pb/idp/thunderid"
)

// defaultConfigPath is used when neither -config nor CONFIG_PATH is set.
const defaultConfigPath = "config.yaml"

// Config is PB's configuration, loaded from a single YAML file.
type Config struct {
	Server ServerConfig `yaml:"server"`
	// Environment names the deployment (e.g. local, staging, production). Log
	// lines carry it when it is set.
	Environment string         `yaml:"environment"`
	Logging     logging.Config `yaml:"logging"`
	CORS        cors.Config    `yaml:"cors"`
	IDP         IDPConfig      `yaml:"idp"`
	PDP         PDPConfig      `yaml:"pdp"`
	Audit       AuditConfig    `yaml:"audit"`
	Database    DatabaseConfig `yaml:"database"`
}

// ServerConfig holds the HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
	// Timeout bounds reading a request and writing its response.
	Timeout time.Duration `yaml:"timeout"`
}

// IDPConfig holds PB's two uses of the identity provider.
type IDPConfig struct {
	// Config validates each incoming token's signature, issuer, audience and expiry.
	authn.Config `yaml:",inline"`
	// Management is PB's own client for ThunderID's management API, which
	// creates each application's OAuth2 client.
	Management thunderid.Config `yaml:"management"`
}

// PDPConfig holds the Policy Decision Point settings.
type PDPConfig struct {
	URL string `yaml:"url"`
}

// AuditConfig holds the Argus audit service settings. Auditing is off when URL is empty.
type AuditConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"apiKey"`
}

// DatabaseConfig is the shared database configuration plus PB's own settings.
type DatabaseConfig struct {
	database.Config `yaml:",inline"`
	// IdleTimeout closes pooled connections that stay idle this long.
	IdleTimeout time.Duration `yaml:"idleTimeout"`
}

// defaultConfig returns the values used for any key the config file leaves out.
func defaultConfig() Config {
	return Config{
		Server:  ServerConfig{Port: 8083, Timeout: 15 * time.Second},
		Logging: logging.DefaultConfig(),
		CORS: cors.Config{
			AllowedOrigins: []string{"http://localhost:5173"},
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowedHeaders: []string{
				"Origin", "Content-Type", "Accept", "Authorization",
				"X-Requested-With", "X-CSRF-Token", "X-Request-ID",
			},
			AllowCredentials: true,
			MaxAge:           86400,
		},
		Database: DatabaseConfig{
			Config: database.Config{
				Driver: database.Postgres,
				Postgres: &database.PostgresConfig{
					Port:    5432,
					Name:    "portal_backend",
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
	flags := flag.NewFlagSet("portal-backend", flag.ContinueOnError)
	path := flags.String("config", configPathFromEnv(), "Path to the YAML config file")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	if err := configyaml.LoadAndExpand(*path, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config file %s: %w", *path, err)
	}
	return &cfg, nil
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
	if err := validation.TCPPort("server.port", c.Server.Port); err != nil {
		return err
	}
	if c.Server.Timeout <= 0 {
		return fmt.Errorf("server.timeout must be positive")
	}
	if err := c.Logging.Validate(); err != nil {
		return err
	}
	if err := c.CORS.Validate(); err != nil {
		return fmt.Errorf("invalid cors configuration: %w", err)
	}
	if err := c.IDP.Validate(); err != nil {
		return fmt.Errorf("invalid idp configuration: %w", err)
	}
	if err := validation.HTTPURL("idp.management.baseURL", c.IDP.Management.BaseURL); err != nil {
		return err
	}
	if c.IDP.Management.ClientID == "" {
		return fmt.Errorf("idp.management.clientID is required")
	}
	if c.IDP.Management.ClientSecret == "" {
		return fmt.Errorf("idp.management.clientSecret is required")
	}
	if err := validation.HTTPURL("pdp.url", c.PDP.URL); err != nil {
		return err
	}
	if c.Audit.URL != "" {
		if err := validation.HTTPURL("audit.url", c.Audit.URL); err != nil {
			return err
		}
	}
	if c.Database.Driver != database.Postgres {
		return fmt.Errorf("database.driver must be postgres")
	}
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}
	pg := c.Database.Postgres
	if err := validation.TCPPort("database.postgres.port", pg.Port); err != nil {
		return err
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
