#!/bin/bash
# Use an installed Go, or bootstrap a verified project-local toolchain.
set -euo pipefail
TASK_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if command -v go >/dev/null 2>&1; then
  TASK_GO="$(command -v go)"
elif [[ -x "$TASK_ROOT/.tools/go/bin/go" ]]; then
  TASK_GO="$TASK_ROOT/.tools/go/bin/go"
elif [[ -x /tmp/ze-process-manager-toolchain/go/bin/go ]]; then
  TASK_GO=/tmp/ze-process-manager-toolchain/go/bin/go
else
  case "$(uname -m)" in
    arm64) TASK_ARCH=arm64; TASK_SHA=76812b213b1b2302c978d28fa52fa92d541704b9e7d9d5db8002c50e4018c4c5 ;;
    x86_64) TASK_ARCH=amd64; TASK_SHA=587b59182488b23aa6e5fc25110405a3e0e5b38ed2f5b2f46ed13c32aee356fe ;;
    *) echo 'Ze AI SkyWatch supports macOS arm64 / amd64.' >&2; exit 1 ;;
  esac
  [[ "$(uname -s)" == Darwin ]] || { echo 'Automatic Go setup requires macOS; install Go to build Ze AI SkyWatch on other platforms.' >&2; exit 1; }
  mkdir -p "$TASK_ROOT/.tools"
  TASK_DOWNLOAD="$(mktemp -d "$TASK_ROOT/.tools/download.XXXXXX")"
  trap 'rm -rf "$TASK_DOWNLOAD"' EXIT
  echo 'Downloading Go 1.27.2 from go.dev…'
  curl --fail --location --retry 3 "https://go.dev/dl/go1.27.2.darwin-$TASK_ARCH.tar.gz" -o "$TASK_DOWNLOAD/go.tar.gz"
  printf '%s  %s\n' "$TASK_SHA" "$TASK_DOWNLOAD/go.tar.gz" | shasum -a 256 -c -
  tar -xzf "$TASK_DOWNLOAD/go.tar.gz" -C "$TASK_ROOT/.tools"
  TASK_GO="$TASK_ROOT/.tools/go/bin/go"
  rm -rf "$TASK_DOWNLOAD"
  trap - EXIT
fi
export PATH="$(dirname "$TASK_GO"):$PATH"
# Session caches are kept in /tmp, including for sandboxed development.
export GOCACHE="${GOCACHE:-/tmp/ze-process-manager-go-cache}"
export GOMODCACHE="${GOMODCACHE:-/tmp/ze-process-manager-go-mod}"
cd "$TASK_ROOT"
exec "$TASK_GO" "$@"
