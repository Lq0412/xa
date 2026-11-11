SHELL := /bin/bash
# WSL 使用原生 Linux Docker 客户端，避免 Windows 客户端误解析挂载路径。
DOCKER ?= /usr/bin/docker
COMPOSE = $(DOCKER) compose --env-file .env -f resources/docker/compose.yaml
export GOTMPDIR := $(CURDIR)/.output/go-tmp
export TMPDIR := $(GOTMPDIR)

.PHONY: env deps-up deps-down deps-status migrate server worker web web-install web-build web-user web-user-install web-user-build api api-client api-docs api-check check integration memory-eval prepare up down status
env:
	bash scripts/init-env.sh

deps-up:
	$(COMPOSE) up -d --wait postgres redis object-store
	bash scripts/run.sh init-storage

deps-down:
	$(COMPOSE) down

deps-status:
	$(COMPOSE) ps -a

server:
	bash scripts/run.sh server

worker:
	bash scripts/run.sh worker

migrate:
	bash scripts/run.sh migrate

web-install:
	cd web && npm install

web:
	cd web && npm run prepare && npm run dev

web-build:
	cd web && npm run build

web-user-install:
	cd web-user && npm install

web-user:
	cd web-user && npm run prepare && npm run dev

web-user-build:
	cd web-user && npm run build

prepare:
	mkdir -p .output/go-tmp

api: prepare
	cd server && go run ./cmd/openapi

api-client: api
	cd web-user && npm run openapi
	cd web && npm run openapi

api-docs: api
	npx --yes @scalar/cli@2.15.0 document serve docs/api/openapi.yaml --watch --port 5177 --config docs/api/scalar.json

api-check: api-client
	git diff --exit-code --stat -- docs/api/openapi.yaml web-user/src/services/generated web/src/services/generated || { echo '接口生成物有变化，请检查并暂存后重跑 make check'; exit 1; }
	test -z "$$(git ls-files --others --exclude-standard -- docs/api/openapi.yaml web-user/src/services/generated web/src/services/generated)" || { echo '请生成并暂存接口约定和客户端'; exit 1; }

check: prepare api-check
	cd server && test -z "$$(gofmt -l .)"
	cd server && go test ./...
	cd server && go vet ./...
	cd server && go build ./...
	cd web && npm run lint
	cd web && npm run build
	cd web-user && npm run lint
	cd web-user && npm run build

integration: prepare
	bash scripts/run.sh migrate
	set -a; source .env; set +a; if [ "$(INTEGRATION_WEB)" = "1" ]; then export XA_BROWSER_INTEGRATION=1; fi; cd server && go test -race -tags=integration -count=1 ./...

memory-eval: prepare
	bash scripts/memory-eval.sh

up:
	bash scripts/dev.sh up

down:
	ALL='$(ALL)' bash scripts/dev.sh down

status:
	bash scripts/dev.sh status
