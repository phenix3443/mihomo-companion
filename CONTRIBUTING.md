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
- add one release label before merge so tagged releases can group notes correctly
- use `enhancement`, `feature`, `change`, or `feat` for `Change`
- use `bug` or `fix` for `Bug Fixes`
- use `chore`, `ci`, `dependencies`, `docs`, `documentation`, `refactor`, or `test` for `Maintenance`
