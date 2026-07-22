.PHONY: db db-stop db-logs run dev up up-d down logs build test vet health

# Load .env if it exists
ifneq (,$(wildcard .env))
  include .env
  export
endif

# ── Local dev (Postgres only, API in shell) ───────────────────────────────

db:           ## Start Postgres in Docker
	docker compose up -d postgres

db-stop:      ## Stop Postgres
	docker compose stop postgres

db-logs:      ## Tail Postgres logs
	docker compose logs -f postgres

run:          ## Run API locally (loads .env, requires db to be up)
	go run ./cmd/api

dev:          ## Start Postgres + run API locally (sequential: db first)
	$(MAKE) db && sleep 2 && $(MAKE) run

# ── Full Docker (Postgres + API both in Docker) ───────────────────────────

up:           ## Build and start everything in Docker (foreground)
	docker compose --profile full up --build

up-d:         ## Build and start everything in Docker (background)
	docker compose --profile full up --build -d

down:         ## Stop and remove all containers
	docker compose --profile full down

logs:         ## Tail all container logs
	docker compose --profile full logs -f

# ── Build & test ──────────────────────────────────────────────────────────

build:        ## Build Go binary → bin/api
	@mkdir -p bin
	go build -o bin/api ./cmd/api

test:         ## Run unit tests
	go test ./...

vet:          ## Run go vet
	go vet ./...

# ── Helpers ───────────────────────────────────────────────────────────────

health:       ## Check /health endpoint
	curl -s http://localhost:8080/health | python3 -m json.tool

help:         ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
