# syntax=docker/dockerfile:1.7
#
# Multi-stage build:
#   1. builder  — Node + Go: runs the Tailwind v4 + DaisyUI v5 CSS build,
#                  generates Templ, compiles the Go binary. The output
#                  is a single static binary; no JS runtime in the
#                  final image.
#   2. runtime  — distroless/static-debian12:nonroot. Just the binary.
#
# The CSS build step (npm install + tailwindcss CLI) is the ONE compile
# that runs in addition to `go build`. It is bounded (~10s on a
# cold cache) and is cached via BuildKit cache mounts. Documented
# in README under "Build pipeline" and in docs/decisions.md.

FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates nodejs npm
WORKDIR /src

# Install Go module deps first (cached independently of source).
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go mod download

# Install CSS build deps (Tailwind v4 CLI + DaisyUI v5).
# Cached via npm's own cache; ~30s on cold cache.
COPY package.json package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm,sharing=locked \
    npm install --no-audit --no-fund

# Now copy the rest of the source and run all the build steps.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    --mount=type=cache,target=/root/.npm,sharing=locked \
    go tool templ generate && \
    npm run build --silent && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /usr/local/bin/app ./cmd/web/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /usr/local/bin/app /app/app
EXPOSE 8080
ENTRYPOINT ["/app/app"]
