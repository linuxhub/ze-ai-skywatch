#!/bin/bash
set -euo pipefail
exec "$(dirname "$0")/go.sh" tool mygo build "$@"
