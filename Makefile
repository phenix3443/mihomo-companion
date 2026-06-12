.PHONY: build install install-yq install-lefthook hooks install-scheduler-deps test lint check-sensitive-links test-sensitive-links ci

build:
	@mkdir -p bin
	@go build -o ./bin/mihctl ./cmd/mihctl

test:
	@go test ./...

lint:
	@test -z "$$(gofmt -l .)" || (echo "Run gofmt on listed files above"; gofmt -l .; exit 1)
	@go vet ./...
	@golangci-lint run ./...

check-sensitive-links:
	@bash scripts/ci/check-no-sensitive-links.sh

test-sensitive-links:
	@bash scripts/ci/check-no-sensitive-links-test.sh

ci: lint test check-sensitive-links test-sensitive-links

install: install-yq install-lefthook hooks install-scheduler-deps

install-yq:
	@scripts/ci/install-yq.sh

install-lefthook:
	@scripts/ci/install-lefthook.sh

hooks:
	@lefthook install

install-scheduler-deps:
	@echo "Installing scheduler dependencies (sing-box, shadowsocks-libev)..."
	@command -v sing-box >/dev/null 2>&1 || brew install sing-box
	@command -v ss-local >/dev/null 2>&1 || brew install shadowsocks-libev
	@echo "Done."
