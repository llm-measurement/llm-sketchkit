#!/usr/bin/env bash
# Code authors: Vijay Erramilli and Codex
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO="${ROOT}/spec/sketches.proto"

if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "protoc-gen-go is required" >&2
  exit 127
fi

python_out="${ROOT}/python"
go_out="${ROOT}"

proto_args=(
  -I "${ROOT}"
  --go_out="${go_out}"
  --go_opt=module=github.com/llm-measurement/llm-sketchkit
  --python_out="${python_out}"
  "${PROTO}"
)

if command -v protoc >/dev/null 2>&1; then
  protoc "${proto_args[@]}"
elif python3 -c 'import grpc_tools.protoc' >/dev/null 2>&1; then
  python3 -m grpc_tools.protoc "${proto_args[@]}"
else
  echo "protoc or python grpc_tools.protoc is required" >&2
  exit 127
fi

python3 - "${ROOT}/go/sketchkit/internal/pb/sketches.pb.go" \
  "${ROOT}/python/spec/sketches_pb2.py" <<'PY'
from pathlib import Path
import sys

author_line = "Code authors: Vijay Erramilli and Codex"
for raw_path in sys.argv[1:]:
    path = Path(raw_path)
    lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
    comment = f"// {author_line}\n" if path.suffix == ".go" else f"# {author_line}\n"
    if comment not in lines:
        lines.insert(1, comment)
        path.write_text("".join(lines), encoding="utf-8")
PY

if [[ "${1:-}" == "--check" ]]; then
  if ! git diff --exit-code -- go/sketchkit/internal/pb python/spec >/dev/null; then
    echo "protobuf generated files are stale; run scripts/regenerate_protos.sh" >&2
    git diff -- go/sketchkit/internal/pb python/spec >&2
    exit 1
  fi
fi
