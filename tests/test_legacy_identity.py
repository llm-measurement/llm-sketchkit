# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Keep Python's v0.2.3 behavior, not the Go legacy normalization quirks."""

import pytest
from llm_sketchkit import canon, hash


@pytest.mark.parametrize(
    ("text", "canonical", "digest"),
    [
        ("\U00011f41\u0301", "f091bd81cc81", "65b636474810928a"),
        ("\U00010041\u0300", "f0908181cc80", "9479d0ac41ebabad"),
    ],
)
def test_legacy_supplementary_identity(
    text: str, canonical: str, digest: str
) -> None:
    value = canon.canonicalize_text_v1(text)
    assert value.hex() == canonical
    assert canon.canonicalize_text_v1(text.encode()) == value
    key = hash.Secret(b"backport-identity-review-synthetic-v1")
    assert hash.digest64_hex(key, hash.PROMPT_V1, value) == digest
