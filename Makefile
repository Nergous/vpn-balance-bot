APP := vpn-balance-bot
PACKAGE := ./cmd/vpn-balance-bot
IMAGE ?= vpn-balance-bot:local
GO ?= go
DOCKER ?= docker
COMPOSE ?= docker compose

.DEFAULT_GOAL := help

.PHONY: help fmt fmt-check tidy-check test test-race vet build verify \
	docker-build docker-test compose-config compose-up compose-down compose-logs

help:
	@printf '%s\n' \
		'make fmt             Format Go source' \
		'make verify          Run repository verification' \
		'make build           Build bin/$(APP)' \
		'make docker-build    Build the runtime image' \
		'make docker-test     Run tests in the Docker build stage' \
		'make compose-up      Build and start the bot' \
		'make compose-down    Stop the bot' \
		'make compose-logs    Follow bot logs'

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)"

tidy-check:
	$(GO) mod tidy -diff

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/$(APP) $(PACKAGE)

verify: fmt-check tidy-check vet test test-race

docker-build:
	$(DOCKER) build --target runtime -t $(IMAGE) .

docker-test:
	$(DOCKER) build --target test .

compose-config:
	$(COMPOSE) config --quiet

compose-up:
	$(COMPOSE) up -d --build

compose-down:
	$(COMPOSE) down

compose-logs:
	$(COMPOSE) logs -f bot
