#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_DIR="${ROOT}/spec"
PROTO="${PROTO_DIR}/sketches.proto"

if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "protoc-gen-go is required" >&2
  exit 127
fi

python_out="${ROOT}/python/llm_sketchkit"
go_out="${ROOT}"

go_proto_args=(
  -I "${ROOT}"
  --go_out="${go_out}"
  --go_opt=module=github.com/llm-measurement/llm-sketchkit
  "${PROTO}"
)

python_proto_args=(
  -I "${PROTO_DIR}"
  --python_out="${python_out}"
  "${PROTO}"
)

if command -v protoc >/dev/null 2>&1; then
  protoc "${go_proto_args[@]}"
  protoc "${python_proto_args[@]}"
elif python3 -c 'import grpc_tools.protoc' >/dev/null 2>&1; then
  python3 -m grpc_tools.protoc "${go_proto_args[@]}"
  python3 -m grpc_tools.protoc "${python_proto_args[@]}"
else
  echo "protoc or python grpc_tools.protoc is required" >&2
  exit 127
fi

python3 - "${ROOT}/go/sketchkit/internal/pb/sketches.pb.go" \
  "${ROOT}/python/llm_sketchkit/sketches_pb2.py" <<'PY'
from pathlib import Path
import sys

spdx_line = "SPDX-License-Identifier: Apache-2.0"
author_line = "Code authors: Vijay Erramilli and Codex"
for raw_path in sys.argv[1:]:
    path = Path(raw_path)
    lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
    marker = "//" if path.suffix == ".go" else "#"
    header = [f"{marker} {spdx_line}\n", f"{marker} {author_line}\n"]
    lines = [line for line in lines if line not in header]
    insert_at = 1 if path.suffix == ".py" and "coding" in lines[0] else 0
    lines[insert_at:insert_at] = header
    path.write_text("".join(lines), encoding="utf-8")
PY

if [[ "${1:-}" == "--check" ]]; then
  if ! git diff --exit-code -- go/sketchkit/internal/pb python/llm_sketchkit/sketches_pb2.py >/dev/null; then
    echo "protobuf generated files are stale; run scripts/regenerate_protos.sh" >&2
    git diff -- go/sketchkit/internal/pb python/llm_sketchkit/sketches_pb2.py >&2
    exit 1
  fi
fi
