.PHONY: help up down destroy logs restart ps token m2m-token ebank-m2m-token jwks open-admin validate oracle-up oracle-down oracle-logs oracle-ps

COMPOSE ?= docker compose
ORACLE_COMPOSE ?= docker compose -f docker-compose.oracle.yml --env-file prod.env
PUBLIC_PORT ?= 8443
ISSUER ?= http://localhost:$(PUBLIC_PORT)/realms/kalke
TOKEN_URL ?= $(ISSUER)/protocol/openid-connect/token
JWKS_URL ?= $(ISSUER)/protocol/openid-connect/certs

# LOCAL COMPOSE ONLY — never reuse these values on auth.kalke.dev.
DEMO_USER ?= demo@kalke.local
DEMO_PASSWORD ?= DemoPass123!
CLI_ID ?= kalke-cli
CLI_SECRET ?= kalke-cli-dev-secret
M2M_ID ?= pde-m2m
M2M_SECRET ?= pde-m2m-dev-secret
EBANK_M2M_ID ?= ebank-m2m
EBANK_M2M_SECRET ?= ebank-m2m-dev-secret

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?##"}; {printf "  %-14s %s\n", $$1, $$2}'

up: ## Start Postgres + Keycloak (internal) + Caddy (public)
	$(COMPOSE) up -d --build
	@echo ""
	@echo "OIDC issuer:  $(ISSUER)"
	@echo "JWKS:         $(JWKS_URL)"
	@echo "Admin UI:     http://localhost:$(PUBLIC_PORT)/admin/ (loopback only)"
	@echo "Demo user:    $(DEMO_USER) / $(DEMO_PASSWORD)"
	@echo "M2M client:   $(M2M_ID) (client_credentials)"

down: ## Stop and remove containers (keeps DB volume)
	$(COMPOSE) down

destroy: ## Stop and delete volumes (re-imports realm on next up)
	$(COMPOSE) down -v

restart: ## Restart stack
	$(COMPOSE) restart

logs: ## Follow logs
	$(COMPOSE) logs -f

ps: ## Show compose status
	$(COMPOSE) ps

jwks: ## Fetch JWKS via the public proxy (not Keycloak internal port)
	curl -fsS "$(JWKS_URL)" | head -c 400; echo

token: ## Dev smoke: password grant → access_token (kalke-cli / demo user)
	@curl -fsS -X POST "$(TOKEN_URL)" \
	  -H 'Content-Type: application/x-www-form-urlencoded' \
	  -d "grant_type=password" \
	  -d "client_id=$(CLI_ID)" \
	  -d "client_secret=$(CLI_SECRET)" \
	  -d "username=$(DEMO_USER)" \
	  -d "password=$(DEMO_PASSWORD)" \
	  -d "scope=openid profile email" | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])'

m2m-token: ## M2M: client_credentials → access_token (pde-m2m)
	@curl -fsS -X POST "$(TOKEN_URL)" \
	  -H 'Content-Type: application/x-www-form-urlencoded' \
	  -d "grant_type=client_credentials" \
	  -d "client_id=$(M2M_ID)" \
	  -d "client_secret=$(M2M_SECRET)" | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])'

ebank-m2m-token: ## M2M: client_credentials → access_token (ebank-m2m)
	@curl -fsS -X POST "$(TOKEN_URL)" \
	  -H 'Content-Type: application/x-www-form-urlencoded' \
	  -d "grant_type=client_credentials" \
	  -d "client_id=$(EBANK_M2M_ID)" \
	  -d "client_secret=$(EBANK_M2M_SECRET)" | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])'

validate: ## Validate realm JSON (same as CI)
	node scripts/validate-realm.mjs

open-admin: ## Print admin console URL
	@echo "http://localhost:$(PUBLIC_PORT)/admin/"

oracle-up: ## Prod on Oracle VM: build + start auth + Caddy (needs prod.env)
	$(ORACLE_COMPOSE) up -d --build
	@echo "Public: https://auth.kalke.dev  (DNS A → this VM)"

oracle-down: ## Stop Oracle prod stack
	$(ORACLE_COMPOSE) down

oracle-logs: ## Follow Oracle prod logs
	$(ORACLE_COMPOSE) logs -f

oracle-ps: ## Show Oracle prod status
	$(ORACLE_COMPOSE) ps
