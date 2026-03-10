APP_NAME    := meowcli
MODULE      := github.com/nekohy/MeowCLI
WEB_DIR     := web
BUILD_DIR   := build

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS     := -s -w \
               -X '$(MODULE)/internal/app.Version=$(VERSION)' \
               -X '$(MODULE)/internal/app.Commit=$(COMMIT)' \
               -X '$(MODULE)/internal/app.BuildTime=$(BUILD_TIME)'

GO          := go
GOFLAGS     ?=
PLATFORMS   := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: all build build-all frontend serve dev-admin lint test sqlc cross docker clean

# ── Default ─────────────────────────────────────────────────
all: build-all

# ── Frontend ────────────────────────────────────────────────
frontend:
	npm --prefix $(WEB_DIR) ci --ignore-scripts
	npm --prefix $(WEB_DIR) run build:ssg

# ── Go build ────────────────────────────────────────────────
build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) .

build-all: frontend build

# ── Dev ─────────────────────────────────────────────────────
serve:
	$(GO) run $(GOFLAGS) -ldflags "$(LDFLAGS)" .

dev-admin:
	npm --prefix $(WEB_DIR) run dev

# ── Codegen ─────────────────────────────────────────────────
sqlc:
	sqlc generate

# ── Cross-compilation ──────────────────────────────────────
cross: frontend
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); arch=$$(echo $$platform | cut -d/ -f2); \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo ">> Building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
			-o $(BUILD_DIR)/$(APP_NAME)-$$os-$$arch$$ext . ; \
	done

# ── Docker ──────────────────────────────────────────────────
docker:
	docker build -t $(APP_NAME):$(VERSION) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) .

# ── Cleanup ─────────────────────────────────────────────────
clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(WEB_DIR)/.nuxt $(WEB_DIR)/.output $(WEB_DIR)/dist
