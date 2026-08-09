package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	App      AppConfig
	DB       DBConfig
	JWT      JWTConfig
	CORS     CORSConfig
	Storage  StorageConfig
	OCR      OCRConfig
	Observab ObservabilityConfig
}

type AppConfig struct {
	Env             string        `envconfig:"APP_ENV" default:"local"`
	Port            int           `envconfig:"APP_PORT" default:"8080"`
	ShutdownTimeout time.Duration `envconfig:"APP_SHUTDOWN_TIMEOUT" default:"15s"`
}

type DBConfig struct {
	Host            string        `envconfig:"DB_HOST" required:"true"`
	Port            int           `envconfig:"DB_PORT" default:"5432"`
	User            string        `envconfig:"DB_USER" required:"true"`
	Password        string        `envconfig:"DB_PASSWORD" required:"true"`
	Name            string        `envconfig:"DB_NAME" required:"true"`
	Schema          string        `envconfig:"DB_SCHEMA" default:"tripmate"`
	SSLMode         string        `envconfig:"DB_SSLMODE" default:"disable"`
	MaxOpenConns    int           `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns    int           `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `envconfig:"DB_CONN_MAX_LIFETIME" default:"30m"`
	AutoMigrate     bool          `envconfig:"DB_AUTO_MIGRATE" default:"false"`
}

type JWTConfig struct {
	AccessSecret  string        `envconfig:"JWT_ACCESS_SECRET"`
	RefreshSecret string        `envconfig:"JWT_REFRESH_SECRET"`
	AccessTTL     time.Duration `envconfig:"JWT_ACCESS_TTL" default:"15m"`
	RefreshTTL    time.Duration `envconfig:"JWT_REFRESH_TTL" default:"720h"`
}

type CORSConfig struct {
	AllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS" default:"http://localhost:3000"`
}

type StorageConfig struct {
	Endpoint  string `envconfig:"STORAGE_ENDPOINT"`
	AccessKey string `envconfig:"STORAGE_ACCESS_KEY"`
	SecretKey string `envconfig:"STORAGE_SECRET_KEY"`
	Bucket    string `envconfig:"STORAGE_BUCKET" default:"tripmate-receipts"`
	Region    string `envconfig:"STORAGE_REGION" default:"us-east-1"`
	UseSSL    bool   `envconfig:"STORAGE_USE_SSL" default:"false"`
	LocalRoot string `envconfig:"STORAGE_LOCAL_ROOT" default:"./var/receipts"`
	PublicURL string `envconfig:"STORAGE_PUBLIC_URL" default:"http://localhost:8080/api/v1/receipt-images"`
}

type OCRConfig struct {
	GeminiAPIKey string        `envconfig:"GEMINI_API_KEY"`
	GeminiModel  string        `envconfig:"GEMINI_MODEL" default:"gemini-1.5-flash"`
	Timeout      time.Duration `envconfig:"OCR_TIMEOUT" default:"30s"`
}

type ObservabilityConfig struct{}

func Load() (*Config, error) {
	environment := os.Getenv("APP_ENV")
	if environment == "" || environment == "local" || environment == "test" {
		_ = godotenv.Load()
	}
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	validEnvironments := map[string]bool{"local": true, "test": true, "staging": true, "production": true}
	if !validEnvironments[c.App.Env] {
		return fmt.Errorf("APP_ENV must be local, test, staging, or production")
	}
	if c.App.Port < 1 || c.App.Port > 65535 {
		return fmt.Errorf("APP_PORT must be between 1 and 65535")
	}
	if c.App.ShutdownTimeout <= 0 {
		return fmt.Errorf("APP_SHUTDOWN_TIMEOUT must be positive")
	}
	if c.DB.MaxOpenConns < 1 || c.DB.MaxIdleConns < 0 || c.DB.MaxIdleConns > c.DB.MaxOpenConns {
		return fmt.Errorf("database pool settings are invalid")
	}
	if c.DB.Schema == "" || strings.ContainsAny(c.DB.Schema, " ;,'\"") {
		return fmt.Errorf("DB_SCHEMA is invalid")
	}
	if c.DB.AutoMigrate && c.App.Env != "local" && c.App.Env != "test" {
		return fmt.Errorf("DB_AUTO_MIGRATE can only be enabled in local or test environments")
	}
	if len(c.JWT.AccessSecret) < 32 || len(c.JWT.RefreshSecret) < 32 {
		return fmt.Errorf("JWT secrets must be at least 32 bytes")
	}
	if c.Storage.Endpoint != "" && (c.Storage.AccessKey == "" || c.Storage.SecretKey == "") {
		return fmt.Errorf("STORAGE_ACCESS_KEY and STORAGE_SECRET_KEY are required with STORAGE_ENDPOINT")
	}
	if c.OCR.Timeout < 0 {
		return fmt.Errorf("OCR_TIMEOUT must be positive")
	}
	if c.IsProduction() {
		if c.DB.SSLMode == "disable" {
			return fmt.Errorf("DB_SSLMODE cannot be disable in production")
		}
		for _, origin := range c.CORS.AllowedOrigins {
			if strings.TrimSpace(origin) == "*" {
				return fmt.Errorf("CORS_ALLOWED_ORIGINS cannot contain * in production")
			}
		}
	}
	return nil
}

func (c *Config) IsProduction() bool { return c.App.Env == "production" }

func (c DBConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s search_path=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode, c.Schema)
}

func (c DBConfig) MigrationURL() string {
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(c.User, c.Password), Host: fmt.Sprintf("%s:%d", c.Host, c.Port), Path: c.Name}
	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	q.Set("search_path", "public")
	u.RawQuery = q.Encode()
	return u.String()
}
