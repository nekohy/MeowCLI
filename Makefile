SHELL := bash
.DEFAULT_GOAL := build

APP_NAME := meowcli
MODULE := github.com/nekohy/MeowCLI
WEB_DIR := web
BUILD_DIR := build
DIST_DIR := dist
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
BACKEND_DEV_ADDR ?= :28582
BACKEND_DEV_URL ?= http://127.0.0.1:28582
FRONTEND_DEV_PORT ?= 39582

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GOFLAGS ?=

LDFLAGS := -s -w \
	-X '$(MODULE)/internal/app.Version=$(VERSION)' \
	-X '$(MODULE)/internal/app.Commit=$(COMMIT)' \
	-X '$(MODULE)/internal/app.BuildTime=$(BUILD_TIME)'

.PHONY: frontend build frontend-dev release clean

frontend:
	npm --prefix $(WEB_DIR) ci --ignore-scripts
	npm --prefix $(WEB_DIR) run build:ssg

build: frontend
	mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) .

frontend-dev:
	@trap 'kill 0' INT TERM EXIT; \
	LISTEN_ADDR=$(BACKEND_DEV_ADDR) go run $(GOFLAGS) -ldflags "$(LDFLAGS)" . & \
	MEOWCLI_BACKEND_URL=$(BACKEND_DEV_URL) npm --prefix $(WEB_DIR) run dev -- --port $(FRONTEND_DEV_PORT)

release: frontend
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)
	for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		out="$(DIST_DIR)/$(APP_NAME)-$$os-$$arch$$ext"; \
		echo ">> $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" -o "$$out" .; \
	done
	cd $(DIST_DIR) && sha256sum $(APP_NAME)-* > checksums-sha256.txt

clean:
	rm -rf $(BUILD_DIR) $(DIST_DIR) $(WEB_DIR)/.nuxt $(WEB_DIR)/.output $(WEB_DIR)/dist
