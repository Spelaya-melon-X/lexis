# syntax=docker/dockerfile:1.7

# ---------- 1. Frontend (Svelte 5 + Vite) ----------
FROM node:22-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm \
    if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY frontend/ ./
RUN npm run build
# результат: /web/dist

# ---------- 2. Backend (Go) ----------
FROM golang:1.27-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY backend/go.mod backend/go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
COPY --from=web /web/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/lexis ./cmd/server

# ---------- 3. Runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lexis /app/lexis
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/lexis"]
