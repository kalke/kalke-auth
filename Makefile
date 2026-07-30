.PHONY: help up down logs restart ps token jwks open-admin

COMPOSE ?= docker compose
PUBLIC_PORT ?= 8443
ISSUER ?= http://localhost:$(PUBLIC_PORT)/realms/kalke
TOKEN_URL ?= $(ISSUER)/protocol/openid-connect/token
JWKS_URL ?= $(ISSUER)/protocol/openid-connect/certs

DEMO_USER ?= demo@kalke.local
DEMO_PASSWORD ?= DemoPass123!
CLI_ID ?= kalke-cli
CLI_SECRET ?= kalke-cli-dev-secret

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?##"}; {printf "  %-14s %s\n", $$1, $$2}'

up: ## Start Postgres + Keycloak (internal) + Caddy (public)
	$(COMPOSE) up -d --build
	@echo ""
	@echo "OIDC issuer:  $(ISSUER)"
	@echo "JWKS:         $(JWKS_URL)"
	@echo "Admin UI:     http://localhost:$(PUBLIC_PORT)/admin/ (loopback only)"
	@echo "Demo user:    $(DEMO_USER) / $(DEMO_PASSWORD)"

down: ## Stop and remove containers
	$(COMPOSE) down

restart: ## Restart stack
	$(COMPOSE) restart

logs: ## Follow logs
	$(COMPOSE) logs -f

ps: ## Show compose status
	$(COMPOSE) ps

jwks: ## Fetch JWKS via the public proxy (not Keycloak internal port)
	curl -fsS "$(JWKS_URL)" | head -c 400; echo

token: ## Dev smoke: password grant → access_token (kalke-cli only)
	@curl -fsS -X POST "$(TOKEN_URL)" \
	  -H 'Content-Type: application/x-www-form-urlencoded' \
	  -d "grant_type=password" \
	  -d "client_id=$(CLI_ID)" \
	  -d "client_secret=$(CLI_SECRET)" \
	  -d "username=$(DEMO_USER)" \
	  -d "password=$(DEMO_PASSWORD)" \
	  -d "scope=openid profile email" | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])'

open-admin: ## Print admin console URL
	@echo "http://localhost:$(PUBLIC_PORT)/admin/"
