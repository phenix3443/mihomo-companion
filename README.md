<p align="center">
  <img src="docs/assets/logo.svg" alt="Mihomo Companion logo" width="128" height="128" />
</p>

<h1 align="center">Mihomo Companion</h1>

<p align="center">
  Companion CLI for managing Mihomo providers and config generation.
</p>

<p align="center">
  <a href="https://github.com/phenix3443/mihctl/actions/workflows/ci.yml">
    <img src="https://github.com/phenix3443/mihctl/actions/workflows/ci.yml/badge.svg" alt="CI status" />
  </a>
  <a href="https://github.com/phenix3443/mihctl/actions/workflows/release.yml">
    <img src="https://github.com/phenix3443/mihctl/actions/workflows/release.yml/badge.svg" alt="Release status" />
  </a>
</p>

`mihctl` is a companion CLI for [Mihomo](https://github.com/MetaCubeX/mihomo). It helps you install Mihomo, manage provider subscriptions, generate config from templates, and sync generated config into a live runtime on macOS and Linux.

When `mihctl` is used from an instance repository, the instance repository provides `config/values.yaml` and `providers/`, while template files are resolved from the public `mihctl` package/repository.

## What this repository contains

- the `mihctl` source code
- `config/mihomo.yaml.tmpl`
- `config/mihomo.service.example`
- `config/values.example.yaml`

## What this repository does not contain

- real provider subscriptions
- real provider YAML files
- generated runtime configs
- private network operations

## Quick start

```bash
make build
cp config/values.example.yaml config/values.yaml
$EDITOR config/values.yaml
./bin/mihctl providers update
./bin/mihctl config gen
```

Tagged releases publish `tar.gz` archives for Linux and macOS on both `amd64` and `arm64`, plus a `checksums.txt` file. Each archive contains `mihctl` and the `config/` template directory.

When `mihctl` is used from an instance repository instead of the `mihctl` source tree, set `MIHCTL_INSTANCE_ROOT` to that instance repository root before running commands. `MIHCTL_INSTANCE_ROOT` points to instance data only; it does not override the public template files bundled with `mihctl`.

## Install with Homebrew

```bash
brew tap phenix3443/tap
brew install mihctl
```

The Homebrew formula installs the `mihctl` binary and ships the tracked `config/` templates under Homebrew's `pkgshare` directory.

Before pushing repository changes, run `make ci`. Keep tracked provider examples on placeholder domains such as `example.com`; real provider subscription links belong only in your local `config/values.yaml`.

## Key commands

```bash
./bin/mihctl config gen
./bin/mihctl config sync --profile local
./bin/mihctl providers update
./bin/mihctl providers sync
./bin/mihctl install
./bin/mihctl service status
```

## Requirements

- Go
- `yq`
- `lefthook`

## Config model

- public templates live in `mihctl/config/`
- track `config/values.example.yaml` in git
- keep your real `config/values.yaml` locally
- fetch provider files into local `providers/`
- generate final configs locally, not as tracked source files

## Supported workflows

- install Mihomo and UI
- update provider subscriptions
- generate runtime config
- sync config and providers into a live Mihomo target
- manage Mihomo service lifecycle

## References

- [mihomo docs](https://wiki.metacubex.one)
- [mihomo sample config](https://github.com/MetaCubeX/mihomo/blob/Meta/docs/config.yaml)
- [metacubexd](https://github.com/MetaCubeX/metacubexd#readme)
