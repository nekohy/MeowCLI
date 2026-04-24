# ── Stage 1: Build frontend ──────────────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build:ssg

# ── Stage 2: Build Go binary ────────────────────────────────
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /src/web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "\
      -s -w \
      -X 'github.com/nekohy/MeowCLI/internal/app.Version=${VERSION}' \
      -X 'github.com/nekohy/MeowCLI/internal/app.Commit=${COMMIT}' \
      -X 'github.com/nekohy/MeowCLI/internal/app.BuildTime=${BUILD_TIME}'" \
    -trimpath \
    -o /out/meowcli .

# ── Stage 3: Minimal runtime ────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/meowcli /usr/local/bin/meowcli

EXPOSE 3000
ENTRYPOINT ["meowcli"]
