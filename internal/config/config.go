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
	Telemetry       TelemetryConfig
	Positions       PositionsConfig
	Push            PushConfig
	LogLevel        string
	ShutdownTimeout time.Duration
}

type PositionsConfig struct {
	CheckInterval time.Duration
	StaleAfter    time.Duration
	Workers       int
	QueueSize     int
}
type PushConfig struct {
	URL            string
	RequestTimeout time.Duration
}

type HTTPConfig struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type TelemetryConfig struct {
	ServiceName string
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

	telemetryConfig, err := loadTelemetryConfig()
	if err != nil {
		return Config{}, err
	}

	positionsConfig, err := loadPositionsConfig()
	if err != nil {
		return Config{}, err
	}
	pushConfig, err := loadPushConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTP:            httpConfig,
		Database:        databaseConfig,
		Telemetry:       telemetryConfig,
		Positions:       positionsConfig,
		Push:            pushConfig,
		LogLevel:        logLevel,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

func loadPositionsConfig() (PositionsConfig, error) {
	checkInterval, err := positiveDuration("POSITION_CHECK_INTERVAL")
	if err != nil {
		return PositionsConfig{}, err
	}

	staleAfter, err := positiveDuration("POSITION_STALE_AFTER")
	if err != nil {
		return PositionsConfig{}, err
	}

	workers, err := parseInt("POSITION_WORKERS")
	if err != nil {
		return PositionsConfig{}, err
	}
	if workers <= 0 {
		return PositionsConfig{}, fmt.Errorf("POSITION_WORKERS must be greater than zero, got %d", workers)
	}

	queueSize, err := parseInt("POSITION_QUEUE_SIZE")
	if err != nil {
		return PositionsConfig{}, err
	}
	if queueSize <= 0 {
		return PositionsConfig{}, fmt.Errorf("POSITION_QUEUE_SIZE must be greater than zero, got %d", queueSize)
	}

	return PositionsConfig{
		CheckInterval: checkInterval,
		StaleAfter:    staleAfter,
		Workers:       workers,
		QueueSize:     queueSize,
	}, nil
}

func loadPushConfig() (PushConfig, error) {
	url, err := required("PUSH_HTTP_URL")
	if err != nil {
		return PushConfig{}, err
	}

	timeout, err := positiveDuration("PUSH_REQUEST_TIMEOUT")
	if err != nil {
		return PushConfig{}, err
	}

	return PushConfig{
		URL:            url,
		RequestTimeout: timeout,
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

// Validates OTEL_* variables for a fail-fast on startup
func loadTelemetryConfig() (TelemetryConfig, error) {
	serviceName, err := required("OTEL_SERVICE_NAME")
	if err != nil {
		return TelemetryConfig{}, err
	}

	if _, err := required("OTEL_EXPORTER_OTLP_ENDPOINT"); err != nil {
		return TelemetryConfig{}, err
	}

	protocol, err := required("OTEL_EXPORTER_OTLP_PROTOCOL")
	if err != nil {
		return TelemetryConfig{}, err
	}
	if protocol != "http/protobuf" {
		return TelemetryConfig{}, fmt.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL must be http/protobuf, got %q", protocol)
	}

	if _, err := required("OTEL_TRACES_SAMPLER"); err != nil {
		return TelemetryConfig{}, err
	}

	return TelemetryConfig{ServiceName: serviceName}, nil
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

func parseInt(name string) (int, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", name, err)
	}
	return number, nil
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
