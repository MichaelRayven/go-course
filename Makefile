.PHONY: migrate generate test run

migrate:
	go tool goose -dir ./migrations postgres "$$DATABASE_URL" up

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

lint: 
	echo "Not implemented"

test: 
	go test -race ./...

run:
	go run ./cmd/trip-service $(ARGS)