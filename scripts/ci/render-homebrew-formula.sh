#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <version> <checksums-file>" >&2
  exit 1
fi

version="$1"
checksums_file="$2"

repo="https://github.com/phenix3443/mihctl"
tag="v${version#v}"
archive_base_url="${repo}/releases/download/${tag}"

sha_for() {
  local artifact="$1"
  awk -v file="$artifact" '$2 == file { print $1 }' "$checksums_file"
}

darwin_amd64="mihctl_${tag}_darwin_amd64.tar.gz"
darwin_arm64="mihctl_${tag}_darwin_arm64.tar.gz"
linux_amd64="mihctl_${tag}_linux_amd64.tar.gz"
linux_arm64="mihctl_${tag}_linux_arm64.tar.gz"

sha_darwin_amd64="$(sha_for "$darwin_amd64")"
sha_darwin_arm64="$(sha_for "$darwin_arm64")"
sha_linux_amd64="$(sha_for "$linux_amd64")"
sha_linux_arm64="$(sha_for "$linux_arm64")"

for value in "$sha_darwin_amd64" "$sha_darwin_arm64" "$sha_linux_amd64" "$sha_linux_arm64"; do
  if [ -z "$value" ]; then
    echo "missing checksum in ${checksums_file}" >&2
    exit 1
  fi
done

cat <<EOF
class Mihctl < Formula
  desc "Companion CLI for managing Mihomo providers and config generation"
  homepage "${repo}"
  version "${tag}"
  license "MIT"

  on_macos do
    on_arm do
      url "${archive_base_url}/${darwin_arm64}"
      sha256 "${sha_darwin_arm64}"
    end
    on_intel do
      url "${archive_base_url}/${darwin_amd64}"
      sha256 "${sha_darwin_amd64}"
    end
  end

  on_linux do
    on_arm do
      url "${archive_base_url}/${linux_arm64}"
      sha256 "${sha_linux_arm64}"
    end
    on_intel do
      url "${archive_base_url}/${linux_amd64}"
      sha256 "${sha_linux_amd64}"
    end
  end

  def install
    bin.install "mihctl"
    pkgshare.install Dir["config/*"]
  end

  test do
    assert_match "Companion CLI for managing Mihomo", shell_output("#{bin}/mihctl --help")
  end
end
EOF
