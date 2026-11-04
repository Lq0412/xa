SHELL := /bin/bash
# WSL 使用原生 Linux Docker 客户端，避免 Windows 客户端误解析挂载路径。
DOCKER ?= /usr/bin/docker
COMPOSE = $(DOCKER) compose --env-file .env -f resources/docker/compose.yaml
export GOTMPDIR := $(CURDIR)/.output/go-tmp
export TMPDIR := $(GOTMPDIR)

.PHONY: env deps-up deps-down deps-status migrate server worker web web-install web-build web-user web-user-install web-user-build check integration prepare
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
	cd web && npm run dev

web-build:
	cd web && npm run build

web-user-install:
	cd web-user && npm install

web-user:
	cd web-user && npm run dev

web-user-build:
	cd web-user && npm run build

prepare:
	mkdir -p .output/go-tmp

check: prepare
	cd server && test -z "$$(gofmt -l .)"
	cd server && go test ./...
	cd server && go vet ./...
	cd server && go build ./...
	cd web && npm run lint
	cd web && npm run lint:antd
	cd web && npm run build
	cd web-user && npm run lint
	cd web-user && npm run lint:antd
	cd web-user && npm run build

integration: prepare
	bash scripts/run.sh migrate
	set -a; source .env; set +a; cd server && go test -race -tags=integration -count=1 ./internal/bootstrap
