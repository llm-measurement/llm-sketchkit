# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Check the exact release bundle before and after draft publication."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any

from scripts.release_metadata import release_tag


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_local(
    directory: Path, version: str, checksums_sha256: str
) -> dict[str, Path]:
    release_tag(version)
    payloads = {
        f"llm_sketchkit-{version}-py3-none-any.whl",
        f"llm_sketchkit-{version}.tar.gz",
        f"llm-sketchkit-{version}.spdx.json",
    }
    files = {path.name: path for path in directory.iterdir()}
    if set(files) != payloads | {"SHA256SUMS"}:
        raise ValueError("release must contain exactly the four expected files")
    if any(path.is_symlink() or not path.is_file() for path in files.values()):
        raise ValueError("release assets must be regular files, not symlinks")
    if sha256(files["SHA256SUMS"]) != checksums_sha256:
        raise ValueError("checksum handoff does not match the assembled artifact")
    lines = files["SHA256SUMS"].read_text(encoding="ascii").splitlines()
    expected = {f"{sha256(files[name])}  {name}" for name in payloads}
    if len(lines) != 3 or set(lines) != expected:
        raise ValueError("checksums must bind exactly the three payload files")
    return files


def validate_remote(
    files: dict[str, Path], metadata: dict[str, Any], tag: str, commit: str, state: str,
    release_id: int | None = None,
) -> None:
    if release_id is not None and metadata.get("id") != release_id:
        raise ValueError("release ID handoff mismatch")
    if metadata.get("tag_name") != tag or metadata.get("target_commitish") != commit:
        raise ValueError("release tag/source handoff mismatch")
    if metadata.get("draft") is not (state == "draft"):
        raise ValueError("unexpected release publication state")
    assets = metadata.get("assets", [])
    if len(assets) != 4 or {asset.get("name") for asset in assets} != set(files):
        raise ValueError("remote release must contain exactly the four expected assets")
    for asset in assets:
        path = files[asset["name"]]
        if (
            asset.get("state") != "uploaded"
            or asset.get("size") != path.stat().st_size
            or asset.get("digest") != f"sha256:{sha256(path)}"
        ):
            raise ValueError(f"remote asset bytes do not match: {path.name}")


def matching_releases(pages: list[Any], tag: str) -> list[dict[str, Any]]:
    releases = [release for page in pages for release in page]
    ids = [release.get("id") for release in releases]
    if (
        any(type(value) is not int or value <= 0 for value in ids)
        or len(set(ids)) != len(ids)
    ):
        raise ValueError("release list contains invalid or duplicate IDs")
    matches = [release for release in releases if release.get("tag_name") == tag]
    if len(matches) > 1:
        raise ValueError("competing releases match the authorized tag")
    return matches


def select_draft(
    files: dict[str, Path], pages: list[Any], tag: str, commit: str
) -> int:
    matches = matching_releases(pages, tag)
    if len(matches) != 1:
        raise ValueError("exactly one release must match the authorized tag")
    metadata = matches[0]
    release_id = int(metadata["id"])
    validate_remote(files, metadata, tag, commit, "draft", release_id)
    return release_id


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", type=Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--checksums-sha256", required=True)
    parser.add_argument("--metadata", type=Path)
    parser.add_argument("--release-list", type=Path)
    parser.add_argument("--require-absent", action="store_true")
    parser.add_argument("--release-id", type=int)
    parser.add_argument("--tag")
    parser.add_argument("--commit")
    parser.add_argument("--state", choices=("draft", "published"))
    args = parser.parse_args()
    files = validate_local(args.directory, args.version, args.checksums_sha256)
    if args.metadata is not None or args.release_list is not None:
        if args.tag != release_tag(args.version) or not args.commit or not args.state:
            parser.error(
                "metadata checks require the matching tag, source commit and state"
            )
    if args.release_list is not None:
        pages = json.loads(args.release_list.read_text())
        if args.require_absent:
            if matching_releases(pages, args.tag):
                raise ValueError(
                    "release already exists; no rebuild or replacement is allowed"
                )
        else:
            print(select_draft(files, pages, args.tag, args.commit))
    if args.metadata is not None:
        if args.release_id is None or args.release_id <= 0:
            parser.error("readback requires a positive release ID")
        validate_remote(
            files, json.loads(args.metadata.read_text()), args.tag, args.commit,
            args.state,
            args.release_id,
        )


if __name__ == "__main__":
    main()
