package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTP            HTTPConfig
	Database        DatabaseConfig
	LogLevel        string
	ShutdownTimeout time.Duration
}

type HTTPConfig struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	QueryTimeout    time.Duration
}

func Load() (Config, error) {
	address, err := required("HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}

	logLevel, err := parseLogLevel()
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := positiveDuration("SHUTDOWN_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	httpConfig, err := loadHTTPConfig(address)
	if err != nil {
		return Config{}, err
	}

	databaseConfig, err := loadDatabaseConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTP:            httpConfig,
		Database:        databaseConfig,
		LogLevel:        logLevel,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

func loadHTTPConfig(address string) (HTTPConfig, error) {
	readTimeout, err := positiveDuration("HTTP_READ_TIMEOUT")
	if err != nil {
		return HTTPConfig{}, err
	}

	readHeaderTimeout, err := positiveDuration("HTTP_READ_HEADER_TIMEOUT")
	if err != nil {
		return HTTPConfig{}, err
	}

	writeTimeout, err := positiveDuration("HTTP_WRITE_TIMEOUT")
	if err != nil {
		return HTTPConfig{}, err
	}

	idleTimeout, err := positiveDuration("HTTP_IDLE_TIMEOUT")
	if err != nil {
		return HTTPConfig{}, err
	}

	return HTTPConfig{
		Addr:              address,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}, nil
}

func loadDatabaseConfig() (DatabaseConfig, error) {
	url, err := required("DATABASE_URL")
	if err != nil {
		return DatabaseConfig{}, err
	}

	maxConns, err := parseInt32("DATABASE_MAX_CONNS")
	if err != nil {
		return DatabaseConfig{}, err
	}
	if maxConns <= 0 {
		return DatabaseConfig{}, fmt.Errorf("DATABASE_MAX_CONNS must be greater than zero")
	}

	minConns, err := parseInt32("DATABASE_MIN_CONNS")
	if err != nil {
		return DatabaseConfig{}, err
	}
	if minConns < 0 {
		return DatabaseConfig{}, fmt.Errorf("DATABASE_MIN_CONNS must not be negative")
	}
	if minConns > maxConns {
		return DatabaseConfig{}, fmt.Errorf("DATABASE_MIN_CONNS must not exceed DATABASE_MAX_CONNS")
	}

	maxConnLifetime, err := positiveDuration("DATABASE_MAX_CONN_LIFETIME")
	if err != nil {
		return DatabaseConfig{}, err
	}

	connectTimeout, err := positiveDuration("DATABASE_CONNECT_TIMEOUT")
	if err != nil {
		return DatabaseConfig{}, err
	}

	queryTimeout, err := positiveDuration("DATABASE_QUERY_TIMEOUT")
	if err != nil {
		return DatabaseConfig{}, err
	}

	return DatabaseConfig{
		URL:             url,
		MaxConns:        maxConns,
		MinConns:        minConns,
		MaxConnLifetime: maxConnLifetime,
		ConnectTimeout:  connectTimeout,
		QueryTimeout:    queryTimeout,
	}, nil
}

func required(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return value, nil
}

func positiveDuration(name string) (time.Duration, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", name, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}

	return duration, nil
}

func parseInt32(name string) (int32, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}

	number, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid 32-bit integer: %w", name, err)
	}

	return int32(number), nil
}

func parseLogLevel() (string, error) {
	value, err := required("LOG_LEVEL")
	if err != nil {
		return "", err
	}

	level := strings.ToLower(value)
	switch level {
	case "debug", "info", "warn", "error":
		return level, nil
	default:
		return "", fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
	}
}
