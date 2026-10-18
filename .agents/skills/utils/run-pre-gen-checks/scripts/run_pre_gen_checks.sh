#!/usr/bin/env bash
#
# run_pre_gen_checks.sh
#
# Wrapper script to execute tools/pre-gen-checks.
# Runs fast Phase 1 pre-generation static checks and unit tests against magic-modules.
# Checks:
# 1. Submodule check (disallows git submodules)
# 2. gofmt formatting check
# 3. tools/template-check (version-guard and unused-tmpl on modified .tmpl files)
# 4. YAML linting check on changed product YAML files (if yamllint is installed)
# 5. mmv1 unit tests
# 6. internal tools & magician unit tests (scoped to touched tools/ or .ci/ files, or all with --all)
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$REPO_ROOT" ] || [ ! -d "${REPO_ROOT}/mmv1" ]; then
  if [ -d "mmv1" ]; then
    REPO_ROOT="$(pwd)"
  else
    echo "Error: This script must be run within the magic-modules repository." >&2
    exit 1
  fi
fi

exec go run -C "${REPO_ROOT}/tools/pre-gen-checks" . "$@"
