.PHONY: dev-api dev-web build test
dev-api:  ## Go API on :8090 using SQLite and seed data
	cd backend && DB_DRIVER=sqlite AUTO_SEED=true go run ./cmd/server
dev-web:  ## Vite dev server on :5173 (proxies /api to :8090)
	cd frontend && npm run dev
build:
	cd frontend && npm ci && npm run build
	cd backend && go build -o bin/server ./cmd/server
test:
	cd backend && go test ./...
	cd frontend && npm run typecheck && npm test
