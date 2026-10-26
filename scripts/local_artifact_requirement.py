#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Emit one hash-locked requirement for a distribution built in this checkout."""

from __future__ import annotations

import argparse
import hashlib
import tomllib
from pathlib import Path


def requirement(artifact: Path, extra: str | None = None) -> str:
    if extra not in (None, "dev"):
        raise ValueError("unsupported local distribution extra")
    artifact = artifact.resolve(strict=True)
    if not artifact.is_file() or not (
        artifact.name.endswith(".whl") or artifact.name.endswith(".tar.gz")
    ):
        raise ValueError("a local wheel or source distribution is required")
    metadata = tomllib.loads(
        (Path(__file__).resolve().parents[1] / "pyproject.toml").read_text()
    )
    name = metadata["project"]["name"]
    with artifact.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    extras = f"[{extra}]" if extra else ""
    return f"{name}{extras} @ {artifact.as_uri()} --hash=sha256:{digest}"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("artifact", type=Path)
    parser.add_argument("--extra", choices=("dev",))
    args = parser.parse_args()
    print(requirement(args.artifact, args.extra))


if __name__ == "__main__":
    main()
