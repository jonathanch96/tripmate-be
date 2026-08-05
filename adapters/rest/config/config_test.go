package config

import (
	"strings"
	"testing"
)

func validConfig() *Config {
	return &Config{
		App:  AppConfig{Env: "local", Port: 8080, ShutdownTimeout: 15_000_000_000},
		DB:   DBConfig{Host: "localhost", Port: 5432, User: "tripmate", Password: "secret", Name: "tripmate", Schema: "tripmate", SSLMode: "disable", MaxOpenConns: 25, MaxIdleConns: 5, ConnMaxLifetime: 1},
		CORS: CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}},
	}
}

func TestValidateRefusesUnsafeProduction(t *testing.T) {
	cfg := validConfig()
	cfg.App.Env = "production"
	cfg.DB.SSLMode = "require"
	cfg.CORS.AllowedOrigins = []string{"*"}
	cfg.JWT.AccessSecret = strings.Repeat("a", 32)
	cfg.JWT.RefreshSecret = strings.Repeat("b", 32)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
		t.Fatalf("got %v", err)
	}
}

func TestDSNIncludesSearchPath(t *testing.T) {
	if dsn := validConfig().DB.DSN(); !strings.Contains(dsn, "search_path=tripmate") {
		t.Fatalf("DSN has no search_path: %s", dsn)
	}
}
