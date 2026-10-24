# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
from __future__ import annotations

import copy
import tempfile
import unittest
from pathlib import Path
from typing import Any

from scripts.release_assets import (
    matching_releases,
    select_draft,
    sha256,
    validate_local,
    validate_remote,
)


class ReleaseAssetsTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.version = "0.5.0"
        self.tag = "v0.5.0"
        self.commit = "a" * 40
        for name, content in {
            "llm_sketchkit-0.5.0-py3-none-any.whl": b"original wheel",
            "llm_sketchkit-0.5.0.tar.gz": b"original sdist",
            "llm-sketchkit-0.5.0.spdx.json": b"{}",
        }.items():
            (self.directory / name).write_bytes(content)
        self.write_checksums()

    def write_checksums(self) -> None:
        paths = sorted(
            path for path in self.directory.iterdir() if path.name != "SHA256SUMS"
        )
        (self.directory / "SHA256SUMS").write_text(
            "".join(f"{sha256(path)}  {path.name}\n" for path in paths)
        )
        self.checksums = sha256(self.directory / "SHA256SUMS")

    def files(self) -> dict[str, Path]:
        return validate_local(self.directory, self.version, self.checksums)

    def metadata(self) -> dict[str, Any]:
        return {
            "id": 701,
            "tag_name": self.tag,
            "target_commitish": self.commit,
            "draft": True,
            "assets": [
                {"name": path.name, "state": "uploaded", "size": path.stat().st_size,
                 "digest": f"sha256:{sha256(path)}"}
                for path in self.files().values()
            ],
        }

    def test_exact_draft_and_published_bundle(self) -> None:
        metadata = self.metadata()
        validate_remote(self.files(), metadata, self.tag, self.commit, "draft")
        metadata["draft"] = False
        validate_remote(self.files(), metadata, self.tag, self.commit, "published")

    def test_changed_wheel_fails(self) -> None:
        (self.directory / "llm_sketchkit-0.5.0-py3-none-any.whl").write_bytes(
            b"rebuilt"
        )
        with self.assertRaisesRegex(ValueError, "checksums"):
            self.files()

    def test_resealed_checksum_handoff_fails(self) -> None:
        original = self.checksums
        (self.directory / "llm_sketchkit-0.5.0.tar.gz").write_bytes(b"rebuilt")
        self.write_checksums()
        with self.assertRaisesRegex(ValueError, "handoff"):
            validate_local(self.directory, self.version, original)

    def test_missing_and_extra_files_fail(self) -> None:
        for name in ("unexpected.whl", "extra-dir"):
            path = self.directory / name
            path.write_bytes(b"extra")
            with self.assertRaisesRegex(ValueError, "exactly"):
                self.files()
            path.unlink()
        (self.directory / "llm-sketchkit-0.5.0.spdx.json").unlink()
        with self.assertRaisesRegex(ValueError, "exactly"):
            self.files()

    def test_symlink_fails(self) -> None:
        path = self.directory / "llm-sketchkit-0.5.0.spdx.json"
        path.unlink()
        path.symlink_to(self.directory / "llm_sketchkit-0.5.0.tar.gz")
        with self.assertRaisesRegex(ValueError, "symlinks"):
            self.files()

    def test_duplicate_checksum_line_fails(self) -> None:
        path = self.directory / "SHA256SUMS"
        path.write_text(path.read_text() + path.read_text().splitlines()[0] + "\n")
        self.checksums = sha256(path)
        with self.assertRaisesRegex(ValueError, "exactly"):
            self.files()

    def test_remote_tag_source_and_state_fail_closed(self) -> None:
        for key, value in (
            ("tag_name", "v9.0.0"), ("target_commitish", "b" * 40), ("draft", False)
        ):
            metadata = self.metadata()
            metadata[key] = value
            with self.subTest(key=key), self.assertRaises(ValueError):
                validate_remote(self.files(), metadata, self.tag, self.commit, "draft")

    def test_remote_missing_duplicate_and_extra_assets_fail(self) -> None:
        metadata = self.metadata()
        for assets in (
            metadata["assets"][:-1], metadata["assets"] + [metadata["assets"][0]],
            metadata["assets"][:-1] + [metadata["assets"][0]]
        ):
            changed = copy.deepcopy(metadata)
            changed["assets"] = assets
            with self.assertRaisesRegex(ValueError, "exactly"):
                validate_remote(self.files(), changed, self.tag, self.commit, "draft")

    def test_remote_bad_digest_size_and_upload_state_fail(self) -> None:
        for key, value in (("digest", "sha256:" + "0" * 64), ("digest", None),
                           ("size", -1), ("state", "new")):
            metadata = self.metadata()
            metadata["assets"][0][key] = value
            with self.subTest(key=key), self.assertRaisesRegex(ValueError, "bytes"):
                validate_remote(self.files(), metadata, self.tag, self.commit, "draft")

    def test_paginated_selection_binds_exact_id(self) -> None:
        pages = [[{"id": 70, "tag_name": "v0.1.0"}], [self.metadata()]]
        self.assertEqual(select_draft(self.files(), pages, self.tag, self.commit), 701)
        validate_remote(
            self.files(), self.metadata(), self.tag, self.commit, "draft", 701
        )

    def test_missing_wrongtag_and_competing_releases_fail(self) -> None:
        metadata = self.metadata()
        wrongtag = {**metadata, "tag_name": "v9.0.0"}
        competing = {**metadata, "id": 702}
        for pages in ([], [[wrongtag]], [[metadata], [competing]]):
            with self.subTest(pages=pages), self.assertRaises(ValueError):
                select_draft(self.files(), pages, self.tag, self.commit)

    def test_duplicate_or_invalid_ids_fail(self) -> None:
        metadata = self.metadata()
        for pages in ([[metadata], [metadata]], [[{**metadata, "id": 0}]],
                      [[{**metadata, "id": "701"}]], [[{**metadata, "id": True}]]):
            with self.assertRaisesRegex(ValueError, "IDs"):
                matching_releases(pages, self.tag)

    def test_id_readback_cannot_rebind_to_another_release(self) -> None:
        metadata = self.metadata()
        metadata["id"] = 702
        with self.assertRaisesRegex(ValueError, "ID handoff"):
            validate_remote(self.files(), metadata, self.tag, self.commit, "draft", 701)


if __name__ == "__main__":
    unittest.main()
