.PHONY: test test-verbose build seed run-identity run-book run-transaction run-worker docker-up docker-down docker-logs clean

test:
	go test ./...

test-verbose:
	go test ./... -v

build:
	go build -o bin/identity-api ./cmd/identity-api
	go build -o bin/book-api ./cmd/book-api
	go build -o bin/transaction-api ./cmd/transaction-api
	go build -o bin/book-worker ./cmd/book-worker

seed:
	go run ./scripts/seed.go

run-identity:
	go run ./cmd/identity-api

run-book:
	go run ./cmd/book-api

run-transaction:
	go run ./cmd/transaction-api

run-worker:
	go run ./cmd/book-worker

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

clean:
	rm -rf bin
