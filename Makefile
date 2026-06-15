.PHONY: run test lint mock tidy

run:
	go run ./cmd/server

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

mock:
	mockery

tidy:
	go mod tidy
