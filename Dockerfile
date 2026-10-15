# --- frontend ---------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- backend ----------------------------------------------------------------
FROM golang:1.26-alpine AS api
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- runtime ----------------------------------------------------------------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app \
 && mkdir -p /app/uploads && chown app /app/uploads
WORKDIR /app
COPY --from=api /out/server /app/server
COPY --from=web /src/frontend/dist /app/web
ENV FRONTEND_DIR=/app/web UPLOAD_DIR=/app/uploads PORT=8080 APP_ENV=production
USER app
EXPOSE 8080
VOLUME ["/app/uploads"]
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/app/server"]
