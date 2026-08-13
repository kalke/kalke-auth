.PHONY: help up down destroy logs restart ps token m2m-token ebank-m2m-token jwks open-admin validate setup aws-up aws-down aws-logs aws-ps fmt test lint

COMPOSE ?= docker compose
AWS_COMPOSE_BASE ?= docker compose -f docker-compose.aws.yml --env-file prod.env
# Tunnel profile is added automatically by aws-up when CLOUDFLARE_TUNNEL_TOKEN is set.
AWS_COMPOSE ?= $(AWS_COMPOSE_BASE)
PUBLIC_PORT ?= 8443
BFF_PORT ?= 8090
ISSUER ?= http://localhost:$(PUBLIC_PORT)/realms/kalke
TOKEN_URL ?= $(ISSUER)/protocol/openid-connect/token
JWKS_URL ?= $(ISSUER)/protocol/openid-connect/certs
BFF_URL ?= http://localhost:$(BFF_PORT)

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

setup: ## Create .env + local.env from examples if missing
	@if [ ! -f .env ]; then cp .env.example .env; echo "Created .env"; else echo ".env already exists"; fi
	@if [ ! -f local.env ]; then cp local.env.example local.env; echo "Created local.env"; else echo "local.env already exists"; fi

up: setup ## Start Keycloak + Caddy + BFF + Postgres + Redis
	$(COMPOSE) up -d --build --wait
	@echo ""
	@echo "OIDC issuer:  $(ISSUER)"
	@echo "JWKS:         $(JWKS_URL)"
	@echo "Auth BFF:     $(BFF_URL)"
	@echo "Admin UI:     http://localhost:$(PUBLIC_PORT)/admin/ (loopback only)"
	@echo "Demo user:    $(DEMO_USER) / $(DEMO_PASSWORD)"
	@echo "M2M client:   $(M2M_ID) (client_credentials)"
	@echo ""
	@echo "Note: realm import only runs on a fresh DB. If kalke-bff is missing, run: make destroy && make up"

down: ## Stop and remove containers (keeps DB volumes)
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

fmt: ## Format Go sources with gofmt
	gofmt -w .

test: ## Run Go tests (same as CI Test job)
	go test ./...

lint: validate ## Match GitHub Actions lint job (vet + gofmt)
	go vet ./...
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
	  echo "Files need gofmt:" >&2; \
	  echo "$$unformatted" >&2; \
	  gofmt -d .; \
	  exit 1; \
	fi

validate: ## Validate realm JSON (same as CI)
	node scripts/validate-realm.mjs

open-admin: ## Print admin console URL
	@echo "http://localhost:$(PUBLIC_PORT)/admin/"

aws-up: ## Prod on AWS Free Tier EC2: build + start auth + Caddy (+ tunnel if token set)
	@tok=$$(awk -F= '/^CLOUDFLARE_TUNNEL_TOKEN=/{sub(/^[^=]*=/,""); gsub(/^["'\'']+|["'\'']+$$/,""); print; exit}' prod.env 2>/dev/null); \
	if [ -n "$$tok" ]; then \
	  $(AWS_COMPOSE_BASE) --profile tunnel up -d --build; \
	  echo "Public: https://auth.kalke.dev"; \
	  echo "Keycloak admin (Access): https://keycloak.kalke.dev/admin/"; \
	else \
	  $(AWS_COMPOSE_BASE) up -d --build; \
	  echo "Public: https://auth.kalke.dev"; \
	  echo "Tip: set CLOUDFLARE_TUNNEL_TOKEN in prod.env for keycloak.kalke.dev"; \
	fi

aws-down: ## Stop AWS prod stack
	$(AWS_COMPOSE_BASE) --profile tunnel down

aws-logs: ## Follow AWS prod logs
	$(AWS_COMPOSE_BASE) --profile tunnel logs -f

aws-ps: ## Show AWS prod stack
	$(AWS_COMPOSE_BASE) --profile tunnel ps
