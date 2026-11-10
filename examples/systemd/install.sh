#!/usr/bin/env bash
# Wrapper: run the full installer from the repository root.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exec "${REPO_ROOT}/install.sh" "$@"
