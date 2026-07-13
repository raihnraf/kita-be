package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServerPort           string
	DBHost               string
	DBPort               string
	DBUser               string
	DBPassword           string
	DBName               string
	DBSSLMode            string
	JWTSecret            string
	JWTExpiry            time.Duration
	RefreshTokenExpiry   time.Duration
	InternalAPIToken     string
	LoanDays             int
	DailyFineAmountCents int64
	MaxActiveBorrows     int
	RabbitMQURL          string
	BookServiceURL       string
}

func Load() (*Config, error) {
	jwtExpiry, err := getEnvDuration("JWT_EXPIRY", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	refreshTokenExpiry, err := getEnvDuration("REFRESH_TOKEN_EXPIRY", 7*24*time.Hour)
	if err != nil {
		return nil, err
	}
	loanDays, err := getEnvInt("LOAN_DAYS", 7)
	if err != nil {
		return nil, err
	}
	dailyFineAmountCents, err := getEnvMoneyCents("DAILY_FINE_AMOUNT", "500.00")
	if err != nil {
		return nil, err
	}
	maxActiveBorrows, err := getEnvInt("MAX_ACTIVE_BORROWS", 3)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ServerPort:           getEnv("SERVER_PORT", "3000"),
		DBHost:               getEnv("DB_HOST", "localhost"),
		DBPort:               getEnv("DB_PORT", "5432"),
		DBUser:               getEnv("DB_USER", "postgres"),
		DBPassword:           getEnv("DB_PASSWORD", "postgres"),
		DBName:               getEnv("DB_NAME", "kita"),
		DBSSLMode:            getEnv("DB_SSLMODE", "disable"),
		JWTSecret:            getEnv("JWT_SECRET", ""),
		JWTExpiry:            jwtExpiry,
		RefreshTokenExpiry:   refreshTokenExpiry,
		InternalAPIToken:     getEnv("INTERNAL_API_TOKEN", ""),
		LoanDays:             loanDays,
		DailyFineAmountCents: dailyFineAmountCents,
		MaxActiveBorrows:     maxActiveBorrows,
		RabbitMQURL:          getEnv("RABBITMQ_URL", ""),
		BookServiceURL:       getEnv("BOOK_SERVICE_URL", ""),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) DatabaseURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

func (c *Config) validate() error {
	if c.ServerPort == "" {
		return fmt.Errorf("SERVER_PORT is required")
	}
	if c.DBHost == "" {
		return fmt.Errorf("DB_HOST is required")
	}
	if c.DBUser == "" {
		return fmt.Errorf("DB_USER is required")
	}
	if c.DBName == "" {
		return fmt.Errorf("DB_NAME is required")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if c.InternalAPIToken == "" {
		return fmt.Errorf("INTERNAL_API_TOKEN is required")
	}
	if c.JWTExpiry <= 0 {
		return fmt.Errorf("JWT_EXPIRY must be positive")
	}
	if c.RefreshTokenExpiry <= 0 {
		return fmt.Errorf("REFRESH_TOKEN_EXPIRY must be positive")
	}
	if c.LoanDays <= 0 {
		return fmt.Errorf("LOAN_DAYS must be positive")
	}
	if c.DailyFineAmountCents < 0 {
		return fmt.Errorf("DAILY_FINE_AMOUNT must be non-negative")
	}
	if c.MaxActiveBorrows <= 0 {
		return fmt.Errorf("MAX_ACTIVE_BORROWS must be positive")
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) (int, error) {
	if val, ok := os.LookupEnv(key); ok {
		i, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer: %w", key, err)
		}
		return i, nil
	}
	return defaultVal, nil
}

func getEnvMoneyCents(key string, defaultVal string) (int64, error) {
	if val, ok := os.LookupEnv(key); ok {
		cents, err := parseMoneyCents(val)
		if err != nil {
			return 0, fmt.Errorf("%s must be a number: %w", key, err)
		}
		return cents, nil
	}
	return parseMoneyCents(defaultVal)
}

func parseMoneyCents(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty value")
	}

	sign := int64(1)
	if strings.HasPrefix(value, "-") {
		sign = -1
		value = strings.TrimPrefix(value, "-")
	}
	if value == "" {
		return 0, fmt.Errorf("missing digits")
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("too many decimal points")
	}

	wholePart := parts[0]
	if wholePart == "" {
		wholePart = "0"
	}
	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil {
		return 0, err
	}

	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if len(fracPart) > 2 {
		return 0, fmt.Errorf("more than two decimal places")
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}

	frac := int64(0)
	if fracPart != "" {
		frac, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return 0, err
		}
	}

	return sign * (whole*100 + frac), nil
}

func getEnvDuration(key string, defaultVal time.Duration) (time.Duration, error) {
	if val, ok := os.LookupEnv(key); ok {
		d, err := time.ParseDuration(val)
		if err != nil {
			return 0, fmt.Errorf("%s must be a duration: %w", key, err)
		}
		return d, nil
	}
	return defaultVal, nil
}
