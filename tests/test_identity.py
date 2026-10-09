# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
from __future__ import annotations

import hashlib
import json
from typing import Any

import pytest
from llm_sketchkit import canon, hash, hllpp, profiles

from scripts.differential_identity import (
    ROOT,
    assert_estimate,
    estimate_case,
    estimate_vectors,
)


def test_unicode15_identity_vectors() -> None:
    vector = json.loads(
        (ROOT / "vectors/identity/text_v1_unicode15.json").read_text(encoding="utf-8")
    )
    assert vector["schema_version"] == 1
    assert vector["unicode_version"] == "15.0.0"
    secret = hash.Secret(vector["secret"].encode())
    for case in vector["cases"]:
        canonical = bytes.fromhex(case["canonical_hex"])
        assert canon.canonicalize_text_v1(case["input"]) == canonical, case["name"]
        assert canon.canonicalize_text_v1(case["input"].encode()) == canonical
        assert canon.canonicalize_text_v1(canonical) == canonical
        assert (
            hash.digest64_hex(secret, profiles.PROMPT_V1, canonical)
            == case["digest_hex"]
        )


@pytest.mark.parametrize("case", estimate_vectors(), ids=lambda case: case["name"])
def test_hllpp_estimate_vectors(case: dict[str, Any]) -> None:
    sketch = estimate_case(case)
    wire = sketch.marshal_binary()
    assert hashlib.sha256(wire).hexdigest() == case["wire_sha256"]
    assert_estimate(sketch.estimate(), case["estimate_bits"], case["max_ulps"])
    parsed = hllpp.parse(wire)
    assert parsed.marshal_binary() == wire
    assert_estimate(parsed.estimate(), case["estimate_bits"], case["max_ulps"])


@pytest.mark.parametrize("text", [b"\xff", b"\xed\xa0\x80", "\ud800", "\udfff"])
def test_canonicalization_keeps_invalid_utf8_errors(text: bytes | str) -> None:
    with pytest.raises(canon.InvalidUTF8Error):
        canon.canonicalize_text_v1(text)


def test_exact_white_space_membership() -> None:
    expected = set(range(0x9, 0xE)) | {
        0x20,
        0x85,
        0xA0,
        0x1680,
        *range(0x2000, 0x200B),
        0x2028,
        0x2029,
        0x202F,
        0x205F,
        0x3000,
    }
    assert set(map(ord, canon._WHITE_SPACE)) == expected
    for codepoint in expected:
        char = chr(codepoint)
        assert canon.canonicalize_text_v1(char + "x" + char) == b"x"
    for codepoint in (0x1C, 0x1D, 0x1E, 0x1F, 0x180E, 0x200B, 0xFEFF):
        char = chr(codepoint)
        assert (
            canon.canonicalize_text_v1(char + "x" + char)
            == (char + "x" + char).encode()
        )


def test_stream_safe_not_a_combining_class_only_scan() -> None:
    for text, expected in (
        ("\u0344" * 16, "\u0308\u0301" * 15 + "\u034f\u0308\u0301"),
        ("\u00a8" + "\u0300" * 30, "\u1fed" + "\u0300" * 28 + "\u034f\u0300"),
        ("\uff9e" * 31, "\uff9e" * 30 + "\u034f\uff9e"),
        ("\uac01" + "\u0300" * 29, "\uac01" + "\u0300" * 28 + "\u034f\u0300"),
        ("a\U00010efd\u0301", "\u00e1\U00010efd"),
        ("a\U0001e08f\u0301", "a\U0001e08f\u0301"),
    ):
        assert canon.canonicalize_text_v1(text) == expected.encode()
