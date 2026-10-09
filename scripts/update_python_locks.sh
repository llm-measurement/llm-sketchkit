#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

for input in requirements/*.in; do
  output="${input%.in}.txt"
  uv pip compile "${input}" \
    --universal --python-version 3.11 --no-python-downloads \
    --generate-hashes --only-binary=:all: \
    --custom-compile-command 'bash scripts/update_python_locks.sh' \
    --output-file "${output}" "$@" > /dev/null
  header="$(mktemp "requirements/.header.XXXXXX")"
  printf '# SPDX-License-Identifier: Apache-2.0\n' > "${header}"
  cat "${output}" >> "${header}"
  mv "${header}" "${output}"
done
