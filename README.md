# Mihomo Companion

`mihctl` is a companion CLI for [Mihomo](https://github.com/MetaCubeX/mihomo). It helps you install Mihomo, manage provider subscriptions, generate config from templates, and sync generated config into a live runtime on macOS and Linux.

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

When `mihctl` is used from an instance repository instead of the `mihomo-companion` source tree, set `MIHCTL_INSTANCE_ROOT` to that instance repository root before running commands.

Before pushing repository changes, run `make ci`. Keep tracked provider examples on placeholder domains such as `example.com`; real provider subscription links belong only in your local `config/values.yaml`.

## Key commands

```bash
./bin/mihctl config gen
./bin/mihctl config sync --profile local
./bin/mihctl providers update
./bin/mihctl providers sync
./bin/mihctl providers probe
./bin/mihctl install
./bin/mihctl service status
```

## Requirements

- Go
- `yq`
- `lefthook`
- for local probe: `sing-box`, `ss-local`
- for some `ss + plugin: obfs` nodes: `simple-obfs`

## Config model

- track `config/values.example.yaml` in git
- keep your real `config/values.yaml` locally
- fetch provider files into local `providers/`
- generate final configs locally, not as tracked source files

## Supported workflows

- install Mihomo and UI
- update provider subscriptions
- probe provider nodes
- generate runtime config
- sync config and providers into a live Mihomo target
- manage Mihomo service lifecycle

## References

- [mihomo docs](https://wiki.metacubex.one)
- [mihomo sample config](https://github.com/MetaCubeX/mihomo/blob/Meta/docs/config.yaml)
- [metacubexd](https://github.com/MetaCubeX/metacubexd#readme)
