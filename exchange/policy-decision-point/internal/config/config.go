// Package config provides simplified configuration management
package config

import (
	"flag"
	"time"

	"github.com/OpenDIF/opendif-core/exchange/shared/utils"
)

// Config holds all configuration for a service
type Config struct {
	Environment string
	Service     ServiceConfig
	Logging     LoggingConfig
	Security    SecurityConfig
	IDPConfig   IDPConfig
	DBConfigs   DBConfigs
}

// ServiceConfig holds service-specific configuration
type ServiceConfig struct {
	Name    string
	Port    string
	Host    string
	Timeout time.Duration
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string
	Format string
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	EnableCORS bool
	RateLimit  int
}

// IDPConfig holds IDP configuration
type IDPConfig struct {
	Issuer   string
	JwksURL  string
	Audience string
}

// DBConfigs holds database configuration
type DBConfigs struct {
	Host     string
	Port     string
	Username string
	Password string
	Database string
	SSLMode  string
}

// LoadConfig loads configuration from flags and environment variables
func LoadConfig(serviceName string) *Config {
	// Get environment first to determine defaults
	env := utils.GetEnvOrDefault("ENVIRONMENT", "local")

	// Define flags
	envFlag := flag.String("env", env, "Environment: local or production")
	port := flag.String("port", utils.GetEnvOrDefault("PORT", "8082"), "Service port")
	host := flag.String("host", utils.GetEnvOrDefault("HOST", "0.0.0.0"), "Host address")
	timeout := flag.Duration("timeout", 10*time.Second, "Request timeout")
	logLevel := flag.String("log-level", getDefaultLogLevel(env), "Log level")
	logFormat := flag.String("log-format", getDefaultLogFormat(env), "Log format")
	enableCORS := flag.Bool("cors", getDefaultCORS(env), "Enable CORS")
	rateLimit := flag.Int("rate-limit", getDefaultRateLimit(env), "Rate limit per minute")

	// Parse flags
	flag.Parse()

	// Reading IDP Configs
	userIssuer := utils.GetEnvOrDefault("IDP_ISSUER", "https://localhost:8090/oauth2")
	userAudience := utils.GetEnvOrDefault("IDP_AUDIENCE", "YOUR_AUDIENCE")
	userJwksURL := utils.GetEnvOrDefault("IDP_JWKS_URL", "https://localhost:8090/oauth2/jwks")

	// Reading DB Configs
	dbHost := utils.GetEnvOrDefault("DB_HOST", "localhost")
	dbPort := utils.GetEnvOrDefault("DB_PORT", "5432")
	dbUsername := utils.GetEnvOrDefault("DB_USERNAME", "postgres")
	dbPassword := utils.GetEnvOrDefault("DB_PASSWORD", "")
	dbName := utils.GetEnvOrDefault("DB_NAME", "pdp")
	dbSslMode := utils.GetEnvOrDefault("DB_SSLMODE", "require")

	// Use flag value if provided, otherwise use environment default
	finalEnv := *envFlag

	config := &Config{
		Environment: finalEnv,
		Service: ServiceConfig{
			Name:    serviceName,
			Port:    *port,
			Host:    *host,
			Timeout: *timeout,
		},
		Logging: LoggingConfig{
			Level:  *logLevel,
			Format: *logFormat,
		},
		Security: SecurityConfig{
			EnableCORS: *enableCORS,
			RateLimit:  *rateLimit,
		},
		IDPConfig: IDPConfig{
			Issuer:   userIssuer,
			JwksURL:  userJwksURL,
			Audience: userAudience,
		},
		DBConfigs: DBConfigs{
			Host:     dbHost,
			Port:     dbPort,
			Username: dbUsername,
			Password: dbPassword,
			Database: dbName,
			SSLMode:  dbSslMode,
		},
	}

	return config
}

func getDefaultLogLevel(env string) string {
	if env == "production" {
		return "warn"
	}
	return "debug"
}

func getDefaultLogFormat(env string) string {
	if env == "production" {
		return "json"
	}
	return "text"
}

func getDefaultCORS(env string) bool {
	return env != "production"
}

func getDefaultRateLimit(env string) int {
	if env == "production" {
		return 100
	}
	return 1000
}
