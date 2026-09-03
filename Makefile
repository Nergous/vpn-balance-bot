APP := vpn-balance-bot
PACKAGE := ./cmd/vpn-balance-bot
IMAGE ?= vpn-balance-bot:local
GO ?= go
DOCKER ?= docker
COMPOSE ?= docker compose
GOLANGCI_LINT_VERSION ?= v2.12.2
GOVULNCHECK_VERSION ?= v1.7.0
ACTIONLINT_VERSION ?= v1.7.12
COVERAGE_FILE ?= .audit-cache/coverage.out
COVERAGE_HTML ?= .audit-cache/coverage.html
COVERAGE_MIN ?= 65.0

.DEFAULT_GOAL := help

.PHONY: help fmt fmt-check tidy-check test test-race vet lint vuln actionlint \
	coverage coverage-check \
	build verify ci version \
	doctor migrate-status \
	docker-build docker-test compose-config compose-up compose-down compose-logs

help:
	@printf '%s\n' \
		'make fmt             Format Go source' \
		'make verify          Run repository verification' \
		'make ci              Run local CI-equivalent checks' \
		'make lint            Run golangci-lint' \
		'make vuln            Scan reachable Go vulnerabilities' \
		'make actionlint      Validate GitHub Actions workflows' \
		'make coverage        Generate Go coverage profile' \
		'make coverage-check  Enforce minimum Go coverage' \
		'make build           Build bin/$(APP)' \
		'make version         Show build information' \
		'make doctor          Check the configured database' \
		'make migrate-status  Show applied and pending migrations' \
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

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

actionlint:
	$(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

coverage:
	mkdir -p $(dir $(COVERAGE_FILE))
	$(GO) test -covermode=atomic -coverprofile=$(COVERAGE_FILE) ./...
	$(GO) tool cover -func=$(COVERAGE_FILE)
	$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)

coverage-check: coverage
	@total="$$($(GO) tool cover -func=$(COVERAGE_FILE) | awk '/^total:/ { gsub(/%/, "", $$3); print $$3 }')"; \
	awk -v total="$$total" -v minimum="$(COVERAGE_MIN)" 'BEGIN { \
		if (total == "" || total + 0 < minimum + 0) { \
			printf "coverage %.1f%% is below %.1f%%\n", total + 0, minimum + 0; \
			exit 1; \
		} \
		printf "coverage %.1f%% meets %.1f%% minimum\n", total + 0, minimum + 0; \
	}'

build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/$(APP) $(PACKAGE)

version:
	$(GO) run $(PACKAGE) version

doctor:
	$(GO) run $(PACKAGE) doctor

migrate-status:
	$(GO) run $(PACKAGE) migrate-status

verify: fmt-check tidy-check vet test test-race

ci: fmt-check tidy-check actionlint vet test-race lint coverage-check

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
