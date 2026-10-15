.PHONY: all build test vet run docker-up docker-down docker-scale load-test clean

all: build

## build: Compile the binary
build:
	go build -o bin/queueforge ./cmd/server

## test: Run unit tests
test:
	go test -v ./...

## vet: Run static code analysis
vet:
	go vet ./...

## run: Run the application locally in all-in-one mode
run:
	go run ./cmd/server

## docker-up: Start all services via Docker Compose
docker-up:
	docker compose up --build -d

## docker-down: Stop all services
docker-down:
	docker compose down

## docker-scale: Scale workers horizontally (e.g., 3 worker instances)
docker-scale:
	docker compose up --scale worker=3 -d

## load-test: Run automated 500-submission load test against localhost:8080
load-test:
	go run ./scripts/load_test.go -url http://localhost:8080 -c 50 -n 500

## clean: Clean compiled binaries
clean:
	rm -rf bin/
