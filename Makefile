.PHONY: run build assets test integration check migrate-up migrate-status

run:
	go run ./cmd/server
build:
	go build -o bin/sim-guru ./cmd/server
assets:
	npm ci
	npm run build
test:
	go test -race ./...
integration:
# Paket memakai database test bersama, jadi sementara dijalankan serial.
	go test -p 1 -count=1 -race -tags=integration ./internal/database ./migrations ./internal/auth ./internal/master ./internal/academic ./internal/web
check:
	test -z "$$(gofmt -l cmd internal migrations)"
	go vet ./...
	git diff --check
migrate-up:
	go run ./cmd/migrate up
migrate-status:
	go run ./cmd/migrate status
