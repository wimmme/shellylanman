# syntax=docker/dockerfile:1
#
# ShellyLanMan: one static Go binary with the frontend embedded, on Alpine.
#
#   docker build -t shellylanman .                 production image
#   docker build --target test .                   all checks (what CI runs)
#
# The web and Go build stages run on the BUILD platform (--platform=$BUILDPLATFORM):
# JavaScript is the same everywhere and Go cross-compiles, so a multi-arch build
# emulates nothing except the few RUN lines of the final stage.

# ── frontend: typecheck, test, bundle ────────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run typecheck && npm test && npm run build

# ── Go sources with the built frontend in place for go:embed ────────────────
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS source
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./internal/web/dist

# ── checks: gofmt, vet, tests with the race detector (needs cgo → Debian image) ─
FROM golang:1.27 AS test
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./internal/web/dist
RUN unformatted="$(gofmt -l .)"; if [ -n "$unformatted" ]; then echo "gofmt needed:"; echo "$unformatted"; exit 1; fi
RUN go vet ./...
RUN go test -race -count=1 ./...

# ── binary ───────────────────────────────────────────────────────────────────
FROM source AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags="-s -w -X github.com/wimmme/shellylanman/internal/version.Version=${VERSION} -X github.com/wimmme/shellylanman/internal/version.Commit=${COMMIT}" \
      -o /out/shellylanman ./cmd/shellylanman

# ── runtime ──────────────────────────────────────────────────────────────────
FROM alpine:3.24
# ca-certificates: HTTPS to Shelly's firmware index; tzdata: local times in logs and UI.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 shellylanman \
 && mkdir /data && chown shellylanman:shellylanman /data
COPY --from=build /out/shellylanman /usr/local/bin/shellylanman
USER shellylanman
VOLUME ["/data"]
EXPOSE 3082
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/shellylanman", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/shellylanman"]
