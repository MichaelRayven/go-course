export DATABASE_URL ?= postgres://tripgo:tripgo@localhost:5432/tripgo?sslmode=disable

.PHONY: migrate run generate

migrate:
	goose -dir ./migrations postgres "$$DATABASE_URL" up

run:
	go run ./cmd/trip-service $(ARGS)

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml