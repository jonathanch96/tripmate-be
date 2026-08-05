package config

import (
	"os"
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

func TestValidateRefusesAutoMigrateOutsideLocal(t *testing.T) {
	cfg := validConfig()
	cfg.App.Env = "staging"
	cfg.DB.AutoMigrate = true
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "DB_AUTO_MIGRATE") {
		t.Fatalf("got %v", err)
	}
}

func TestDSNIncludesSearchPath(t *testing.T) {
	if dsn := validConfig().DB.DSN(); !strings.Contains(dsn, "search_path=tripmate") {
		t.Fatalf("DSN has no search_path: %s", dsn)
	}
}

func TestLoadNamesMissingRequiredVariable(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "test")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_USER", "tripmate")
	t.Setenv("DB_NAME", "tripmate")
	previous, existed := os.LookupEnv("DB_PASSWORD")
	if err := os.Unsetenv("DB_PASSWORD"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("DB_PASSWORD", previous)
		} else {
			_ = os.Unsetenv("DB_PASSWORD")
		}
	})

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Fatalf("error must name DB_PASSWORD, got %v", err)
	}
}
