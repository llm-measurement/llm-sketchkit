# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Validate the Python package version against the public release tag."""

from __future__ import annotations

import argparse
import re
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
VERSION_PATTERN = re.compile(
    r"(?P<release>[0-9]+(?:\.[0-9]+)+)"
    r"(?:(?P<phase>a|b|rc)(?P<number>[0-9]+))?"
)
PHASE_NAMES = {"a": "alpha", "b": "beta", "rc": "rc"}


def project_version() -> str:
    data = tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))
    return str(data["project"]["version"])


def release_tag(version: str) -> str:
    match = VERSION_PATTERN.fullmatch(version)
    if match is None:
        raise ValueError(f"unsupported release version: {version!r}")

    tag = f"v{match.group('release')}"
    phase = match.group("phase")
    if phase is not None:
        tag += f"-{PHASE_NAMES[phase]}.{match.group('number')}"
    return tag


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tag", required=True)
    parser.add_argument("--github-output", type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    version = project_version()
    expected_tag = release_tag(version)
    if args.tag != expected_tag:
        raise SystemExit(f"release tag {args.tag!r} != {expected_tag!r}")

    if args.github_output is not None:
        with args.github_output.open("a", encoding="utf-8") as output:
            print(f"python_version={version}", file=output)
            print(f"go_version={args.tag}", file=output)


if __name__ == "__main__":
    main()
