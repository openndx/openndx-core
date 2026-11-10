package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/OpenNSW/core/configyaml"
	"github.com/OpenNSW/core/database"
	"github.com/openndx/openndx-core/internal/logging"
)

// defaultConfigPath is used when neither -config nor CONFIG_PATH is set.
const defaultConfigPath = "config.yaml"

// Config is the PDP's configuration, loaded from a single YAML file.
type Config struct {
	Server ServerConfig `yaml:"server"`
	// Environment names the deployment (e.g. local, staging, production). Log
	// lines carry it when it is set.
	Environment string         `yaml:"environment"`
	Logging     logging.Config `yaml:"logging"`
	Database    DatabaseConfig `yaml:"database"`
	Policy      PolicyConfig   `yaml:"policy"`
}

// PolicyConfig holds the policy engine settings.
type PolicyConfig struct {
	// ModulesDir is a directory of custom policy classes, compiled with the
	// built-in ones at startup. Empty loads the built-in classes only.
	ModulesDir string `yaml:"modulesDir"`
}

// ServerConfig holds the HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
	// Timeout bounds reading a request and writing its response.
	Timeout time.Duration `yaml:"timeout"`
}

// DatabaseConfig is the shared database configuration plus the PDP's own settings.
type DatabaseConfig struct {
	database.Config `yaml:",inline"`
	// IdleTimeout closes pooled connections that stay idle this long.
	IdleTimeout time.Duration `yaml:"idleTimeout"`
}

// defaultConfig returns the values used for any key the config file leaves out.
func defaultConfig() Config {
	return Config{
		Server:  ServerConfig{Port: 8082, Timeout: 10 * time.Second},
		Logging: logging.DefaultConfig(),
		Database: DatabaseConfig{
			Config: database.Config{
				Driver: database.Postgres,
				Postgres: &database.PostgresConfig{
					Port:    5432,
					Name:    "pdp",
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
	flags := flag.NewFlagSet("policy-decision-point", flag.ContinueOnError)
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
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if c.Server.Timeout <= 0 {
		return fmt.Errorf("server.timeout must be positive")
	}
	if err := c.Logging.Validate(); err != nil {
		return err
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
