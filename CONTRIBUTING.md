# Contributing

## Development

- `make build`
- `make lint`
- `make test`
- `make check-sensitive-links`
- `make test-sensitive-links`
- `make ci`
- `go test ./...`

## Rules

- do not commit `config/values.yaml`
- do not commit `providers/*.yaml`
- keep `config/values.example.yaml` provider URLs on placeholder domains such as `example.com`
- do not commit real provider subscription links or proxy scheme links such as `ss://` or `trojan://`
- do not commit generated runtime configs
- do not add scenario-specific private operations to the public CLI

## Pull requests

- keep changes focused on one logical change
- add or update tests for behavior changes
- update docs when command behavior or config expectations change
