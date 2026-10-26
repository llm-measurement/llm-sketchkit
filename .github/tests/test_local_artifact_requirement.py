# SPDX-License-Identifier: Apache-2.0
from __future__ import annotations

import hashlib
import importlib.util
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "local_artifact_requirement", ROOT / "scripts/local_artifact_requirement.py"
)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


@pytest.mark.parametrize("suffix,extra", [(".whl", None), (".tar.gz", "dev")])
def test_hash_binds_exact_local_artifact(
    tmp_path: Path, suffix: str, extra: str | None
) -> None:
    artifact = tmp_path / f"local distribution{suffix}"
    artifact.write_bytes(b"local distribution bytes")
    expected = hashlib.sha256(artifact.read_bytes()).hexdigest()
    name = "llm-sketchkit[dev]" if extra else "llm-sketchkit"
    assert MODULE.requirement(artifact, extra) == (
        f"{name} @ {artifact.as_uri()} --hash=sha256:{expected}"
    )


def test_changed_artifact_changes_requirement(tmp_path: Path) -> None:
    artifact = tmp_path / "local.whl"
    artifact.write_bytes(b"first")
    before = MODULE.requirement(artifact)
    artifact.write_bytes(b"second")
    assert MODULE.requirement(artifact) != before


def test_missing_or_unsupported_artifact_fails(tmp_path: Path) -> None:
    with pytest.raises(FileNotFoundError):
        MODULE.requirement(tmp_path / "missing.whl")
    artifact = tmp_path / "remote.txt"
    artifact.write_text("untrusted")
    with pytest.raises(ValueError):
        MODULE.requirement(artifact)
    with pytest.raises(ValueError):
        MODULE.requirement(artifact, "other")
