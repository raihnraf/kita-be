package config_test

import (
	"strings"
	"testing"
	"time"

	"kita-be/internal/platform/config"
)

func TestLoadValidConfig(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.ServerPort != "3000" {
		t.Errorf("expected ServerPort 3000, got %s", cfg.ServerPort)
	}
	if cfg.DBHost != "localhost" {
		t.Errorf("expected DBHost localhost, got %s", cfg.DBHost)
	}
	if cfg.JWTSecret != "test-secret-with-at-least-32-chars" {
		t.Errorf("unexpected JWTSecret: %s", cfg.JWTSecret)
	}
	if cfg.JWTExpiry != 15*time.Minute {
		t.Errorf("expected JWTExpiry 15m, got %s", cfg.JWTExpiry)
	}
	if cfg.DatabaseURL() != "postgres://postgres:postgres@localhost:5432/kita?sslmode=disable" {
		t.Errorf("unexpected database URL: %s", cfg.DatabaseURL())
	}
	if cfg.DailyFineAmountCents != 50000 {
		t.Errorf("expected DailyFineAmountCents 50000, got %d", cfg.DailyFineAmountCents)
	}
}

func TestLoadInvalidConfigMissingJWTSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_SECRET", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing JWT_SECRET, got nil")
	}
}

func TestLoadInvalidConfigShortJWTSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_SECRET", "short-secret")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for short JWT_SECRET, got nil")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET must be at least 32 characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadInvalidConfigLowDiversityJWTSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for low-diversity JWT_SECRET, got nil")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET must contain enough character diversity") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadInvalidConfigMissingInternalToken(t *testing.T) {
	setValidEnv(t)
	t.Setenv("INTERNAL_API_TOKEN", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing INTERNAL_API_TOKEN, got nil")
	}
}

func TestLoadRejectsMalformedEnvValues(t *testing.T) {
	cases := []struct {
		key      string
		value    string
		contains string
	}{
		{key: "JWT_EXPIRY", value: "soon", contains: "JWT_EXPIRY must be a duration"},
		{key: "LOAN_DAYS", value: "seven", contains: "LOAN_DAYS must be an integer"},
		{key: "DAILY_FINE_AMOUNT", value: "many", contains: "DAILY_FINE_AMOUNT must be a number"},
		{key: "MAX_ACTIVE_BORROWS", value: "three", contains: "MAX_ACTIVE_BORROWS must be an integer"},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv(tc.key, tc.value)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error for %s", tc.key)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected error containing %q, got %q", tc.contains, err.Error())
			}
		})
	}
}

func TestLoadRejectsInvalidRanges(t *testing.T) {
	cases := []struct {
		key      string
		value    string
		contains string
	}{
		{key: "JWT_EXPIRY", value: "0s", contains: "JWT_EXPIRY must be positive"},
		{key: "REFRESH_TOKEN_EXPIRY", value: "0s", contains: "REFRESH_TOKEN_EXPIRY must be positive"},
		{key: "LOAN_DAYS", value: "0", contains: "LOAN_DAYS must be positive"},
		{key: "DAILY_FINE_AMOUNT", value: "-1", contains: "DAILY_FINE_AMOUNT must be non-negative"},
		{key: "MAX_ACTIVE_BORROWS", value: "0", contains: "MAX_ACTIVE_BORROWS must be positive"},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv(tc.key, tc.value)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error for %s", tc.key)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected error containing %q, got %q", tc.contains, err.Error())
			}
		})
	}
}

func TestLoadParsesDailyFineAmountAsCents(t *testing.T) {
	cases := []struct {
		value string
		want  int64
	}{
		{value: "500", want: 50000},
		{value: "500.0", want: 50000},
		{value: "500.05", want: 50005},
		{value: ".50", want: 50},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("DAILY_FINE_AMOUNT", tc.value)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if cfg.DailyFineAmountCents != tc.want {
				t.Fatalf("expected %d cents, got %d", tc.want, cfg.DailyFineAmountCents)
			}
		})
	}
}

func TestLoadRejectsDailyFineAmountWithMoreThanTwoDecimals(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DAILY_FINE_AMOUNT", "1.234")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for too many decimal places")
	}
	if !strings.Contains(err.Error(), "DAILY_FINE_AMOUNT must be a number") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsDevelopmentSecretsInProduction(t *testing.T) {
	cases := []struct {
		key      string
		value    string
		contains string
	}{
		{key: "JWT_SECRET", value: "dev-jwt-secret-change-in-production", contains: "JWT_SECRET must not use the development default in production"},
		{key: "INTERNAL_API_TOKEN", value: "dev-internal-token", contains: "INTERNAL_API_TOKEN must not use the development default in production"},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("APP_ENV", "production")
			t.Setenv(tc.key, tc.value)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error for %s", tc.key)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected error containing %q, got %q", tc.contains, err.Error())
			}
		})
	}
}

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "test")
	t.Setenv("SERVER_PORT", "3000")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "postgres")
	t.Setenv("DB_PASSWORD", "postgres")
	t.Setenv("DB_NAME", "kita")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("JWT_SECRET", "test-secret-with-at-least-32-chars")
	t.Setenv("JWT_EXPIRY", "15m")
	t.Setenv("REFRESH_TOKEN_EXPIRY", "168h")
	t.Setenv("INTERNAL_API_TOKEN", "test-internal-token")
	t.Setenv("LOAN_DAYS", "7")
	t.Setenv("DAILY_FINE_AMOUNT", "500.0")
	t.Setenv("MAX_ACTIVE_BORROWS", "3")
}
