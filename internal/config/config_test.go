package config_test

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/MichaelRayven/go-course/internal/config"
)

func validEnv() map[string]string {
	return map[string]string{
		"HTTP_ADDR":                   ":8080",
		"LOG_LEVEL":                   "info",
		"SHUTDOWN_TIMEOUT":            "10s",
		"HTTP_READ_TIMEOUT":           "3s",
		"HTTP_READ_HEADER_TIMEOUT":    "3s",
		"HTTP_WRITE_TIMEOUT":          "5s",
		"HTTP_IDLE_TIMEOUT":           "10s",
		"DATABASE_URL":                "postgres://tripgo:tripgo@localhost:5432/tripgo?sslmode=disable",
		"DATABASE_MAX_CONNS":          "10",
		"DATABASE_MIN_CONNS":          "2",
		"DATABASE_MAX_CONN_LIFETIME":  "30m",
		"DATABASE_CONNECT_TIMEOUT":    "5s",
		"DATABASE_QUERY_TIMEOUT":      "3s",
		"OTEL_SERVICE_NAME":           "trip-service",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318",
		"OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
		"OTEL_TRACES_SAMPLER":         "parentbased_always_on",
		"POSITION_CHECK_INTERVAL":     "30s",
		"POSITION_STALE_AFTER":        "60s",
		"POSITION_WORKERS":            "4",
		"POSITION_QUEUE_SIZE":         "100",
		"PUSH_HTTP_URL":               "http://localhost:8090",
		"PUSH_REQUEST_TIMEOUT":        "1s",
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		modifyEnv   func(env map[string]string)
		wantErr     bool
		errContains string
		checkConfig func(t *testing.T, cfg config.Config)
	}{
		{
			name: "success with valid env",
			modifyEnv: func(env map[string]string) {
				// Используем валидный базовый набор
			},
			wantErr: false,
			checkConfig: func(t *testing.T, cfg config.Config) {
				if cfg.HTTP.Addr != ":8080" {
					t.Errorf("expected HTTP.Addr :8080, got %s", cfg.HTTP.Addr)
				}
				if cfg.Positions.CheckInterval != 30*time.Second {
					t.Errorf("expected CheckInterval 30s, got %v", cfg.Positions.CheckInterval)
				}
				if cfg.Positions.StaleAfter != 60*time.Second {
					t.Errorf("expected StaleAfter 60s, got %v", cfg.Positions.StaleAfter)
				}
				if cfg.Positions.Workers != 4 {
					t.Errorf("expected Workers 4, got %d", cfg.Positions.Workers)
				}
				if cfg.Positions.QueueSize != 100 {
					t.Errorf("expected QueueSize 100, got %d", cfg.Positions.QueueSize)
				}
				if cfg.Push.URL != "http://localhost:8090" {
					t.Errorf("expected Push.URL http://localhost:8090, got %s", cfg.Push.URL)
				}
				if cfg.Push.RequestTimeout != 1*time.Second {
					t.Errorf("expected Push.RequestTimeout 1s, got %v", cfg.Push.RequestTimeout)
				}
			},
		},
		{
			name: "missing POSITION_CHECK_INTERVAL",
			modifyEnv: func(env map[string]string) {
				delete(env, "POSITION_CHECK_INTERVAL")
			},
			wantErr:     true,
			errContains: "POSITION_CHECK_INTERVAL is required",
		},
		{
			name: "invalid duration format for POSITION_CHECK_INTERVAL",
			modifyEnv: func(env map[string]string) {
				env["POSITION_CHECK_INTERVAL"] = "thirty_seconds"
			},
			wantErr:     true,
			errContains: "must be a valid duration",
		},
		{
			name: "zero POSITION_CHECK_INTERVAL",
			modifyEnv: func(env map[string]string) {
				env["POSITION_CHECK_INTERVAL"] = "0s"
			},
			wantErr:     true,
			errContains: "must be greater than zero",
		},
		{
			name: "negative POSITION_STALE_AFTER",
			modifyEnv: func(env map[string]string) {
				env["POSITION_STALE_AFTER"] = "-10s"
			},
			wantErr:     true,
			errContains: "must be greater than zero",
		},
		{
			name: "zero POSITION_WORKERS",
			modifyEnv: func(env map[string]string) {
				env["POSITION_WORKERS"] = "0"
			},
			wantErr:     true,
			errContains: "POSITION_WORKERS must be greater than zero",
		},
		{
			name: "negative POSITION_WORKERS",
			modifyEnv: func(env map[string]string) {
				env["POSITION_WORKERS"] = "-2"
			},
			wantErr:     true,
			errContains: "POSITION_WORKERS must be greater than zero",
		},
		{
			name: "non-integer POSITION_WORKERS",
			modifyEnv: func(env map[string]string) {
				env["POSITION_WORKERS"] = "four"
			},
			wantErr:     true,
			errContains: "must be a valid integer",
		},
		{
			name: "zero POSITION_QUEUE_SIZE",
			modifyEnv: func(env map[string]string) {
				env["POSITION_QUEUE_SIZE"] = "0"
			},
			wantErr:     true,
			errContains: "POSITION_QUEUE_SIZE must be greater than zero",
		},
		{
			name: "missing PUSH_HTTP_URL",
			modifyEnv: func(env map[string]string) {
				delete(env, "PUSH_HTTP_URL")
			},
			wantErr:     true,
			errContains: "PUSH_HTTP_URL is required",
		},
		{
			name: "zero PUSH_REQUEST_TIMEOUT",
			modifyEnv: func(env map[string]string) {
				env["PUSH_REQUEST_TIMEOUT"] = "0s"
			},
			wantErr:     true,
			errContains: "must be greater than zero",
		},
		{
			name: "invalid LOG_LEVEL",
			modifyEnv: func(env map[string]string) {
				env["LOG_LEVEL"] = "verbose"
			},
			wantErr:     true,
			errContains: "LOG_LEVEL must be one of debug, info, warn, error",
		},
		{
			name: "DATABASE_MIN_CONNS exceeds DATABASE_MAX_CONNS",
			modifyEnv: func(env map[string]string) {
				env["DATABASE_MAX_CONNS"] = "2"
				env["DATABASE_MIN_CONNS"] = "5"
			},
			wantErr:     true,
			errContains: "DATABASE_MIN_CONNS must not exceed DATABASE_MAX_CONNS",
		},
		{
			name: "invalid OTEL_EXPORTER_OTLP_PROTOCOL",
			modifyEnv: func(env map[string]string) {
				env["OTEL_EXPORTER_OTLP_PROTOCOL"] = "grpc"
			},
			wantErr:     true,
			errContains: "must be http/protobuf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envCopy := maps.Clone(validEnv())
			tt.modifyEnv(envCopy)

			for k, v := range envCopy {
				t.Setenv(k, v)
			}
			// Для удаленных ключей явно сбрасываем переменную в пустую строку
			for k := range validEnv() {
				if _, ok := envCopy[k]; !ok {
					t.Setenv(k, "")
				}
			}

			cfg, err := config.Load()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			}

			if !tt.wantErr && tt.checkConfig != nil {
				tt.checkConfig(t, cfg)
			}
		})
	}
}
