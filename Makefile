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
DOCKER_IMAGE ?= meowcli:latest
RELEASE_OS ?= $(shell go env GOOS)
RELEASE_ARCH ?= $(shell go env GOARCH)
RELEASE_EXT := $(if $(filter windows,$(RELEASE_OS)),.exe,)
RELEASE_BINARY := $(DIST_DIR)/$(APP_NAME)-$(RELEASE_OS)-$(RELEASE_ARCH)$(RELEASE_EXT)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GOFLAGS ?=

LDFLAGS := -s -w \
	-X '$(MODULE)/internal/app.Version=$(VERSION)' \
	-X '$(MODULE)/internal/app.BuildTime=$(BUILD_TIME)'

.PHONY: frontend build frontend-dev release checksums docker clean

frontend:
	npm --prefix $(WEB_DIR) ci --ignore-scripts
	npm --prefix $(WEB_DIR) run build:ssg

build: frontend
	mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) .

frontend-dev:
	@trap 'kill 0' INT TERM EXIT; \
	LISTEN_ADDR=$(BACKEND_DEV_ADDR) go run $(GOFLAGS) -tags dev -ldflags "$(LDFLAGS)" . & \
	MEOWCLI_BACKEND_URL=$(BACKEND_DEV_URL) npm --prefix $(WEB_DIR) run dev -- --port $(FRONTEND_DEV_PORT)

release:
	@test -d "$(WEB_DIR)/dist" || { echo "missing $(WEB_DIR)/dist; run make frontend first"; exit 1; }
	mkdir -p $(DIST_DIR)
	@echo ">> $(RELEASE_BINARY)"
	CGO_ENABLED=0 GOOS=$(RELEASE_OS) GOARCH=$(RELEASE_ARCH) \
		go build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" -o "$(RELEASE_BINARY)" .

checksums:
	@test -d "$(DIST_DIR)" || { echo "missing $(DIST_DIR); run make release first"; exit 1; }
	cd $(DIST_DIR) && sha256sum $(APP_NAME)-* > checksums-sha256.txt

docker:
	docker build \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILD_TIME="$(BUILD_TIME)" \
		-t "$(DOCKER_IMAGE)" .

clean:
	rm -rf $(BUILD_DIR) $(DIST_DIR) $(WEB_DIR)/.nuxt $(WEB_DIR)/.output $(WEB_DIR)/dist
