.PHONY: migrate generate test run lint fmt

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

test: 
	go test -race ./...

run:
	go run ./cmd/trip-service $(ARGS)