# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
from __future__ import annotations

import pytest

from scripts.release_metadata import release_tag


@pytest.mark.parametrize(
    ("version", "tag"),
    [
        ("0.1.0", "v0.1.0"),
        ("0.2.0a1", "v0.2.0-alpha.1"),
        ("0.2.0b2", "v0.2.0-beta.2"),
        ("1.0.0rc3", "v1.0.0-rc.3"),
    ],
)
def test_release_tag(version: str, tag: str) -> None:
    assert release_tag(version) == tag


@pytest.mark.parametrize("version", ["0.1.0.dev1", "0.1.0.post1", "not-a-version"])
def test_release_tag_rejects_unsupported_versions(version: str) -> None:
    with pytest.raises(ValueError, match="unsupported release version"):
        release_tag(version)
