APP   := hls-downloader
IMAGE := hls-downloader

.PHONY: all build run fmt vet test clean \
        docker-build docker-up docker-down docker-logs help

all: build

## build: compile the binary into ./hls-downloader
build:
	go build -o $(APP) ./cmd

## run: build and start the server on :6969 (override output: OUTPUT_DIR=~/Movies make run)
run: build
	./$(APP)

## fmt: format all Go files
fmt:
	gofmt -w .

## vet: run go vet
vet:
	go vet ./...

## test: run all tests
test:
	go test ./...

## clean: remove the built binary
clean:
	rm -f $(APP)

## docker-build: build the Docker image
docker-build:
	docker build -t $(IMAGE) .

## docker-up: build and start via docker compose (detached)
docker-up:
	docker compose up --build -d

## docker-down: stop the compose stack
docker-down:
	docker compose down

## docker-logs: tail container logs
docker-logs:
	docker compose logs -f

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
