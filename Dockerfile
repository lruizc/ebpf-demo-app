# syntax=docker/dockerfile:1

# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

ARG VERSION=dev

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /weather-app \
      ./cmd/weather-app

# ── Final stage ────────────────────────────────────────────────────────────────
# distroless/static: no shell, no package manager, no libc — minimal attack surface
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /weather-app /weather-app

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/weather-app"]
