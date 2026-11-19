# Local addresses from `tripgoctl environment start`; ignored if missing.
-include .env
export

.PHONY: migrate generate test test-integration run lint fmt

migrate:
	go tool goose -dir ./migrations postgres "$$DATABASE_URL" up

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml
	go generate ./...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

COVERPKG := ./internal/domain/...,./internal/usecase/...

test:
	go test -race -count=1 -coverpkg=$(COVERPKG) -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -n 1

test-integration:
	go test -race -count=1 -tags integration ./internal/repository/...

run:
	go run ./cmd/trip-service $(ARGS)