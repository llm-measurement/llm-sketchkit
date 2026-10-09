# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex

import traceback

import pytest
from llm_sketchkit import canon


@pytest.mark.parametrize("profile,value", [
    ("sentinel-private-profile\n", b"valid"),
    ("text_v1", b"sentinel-private-value\xff"),
    ("text_v1", "sentinel-private-value\udc80"),
])
def test_canonicalization_errors_hide_inputs(profile: str, value: bytes | str) -> None:
    try:
        canon.canonicalize(profile, value)
    except canon.CanonicalizationError as exc:
        assert "sentinel-private" not in str(exc)
        assert "sentinel-private" not in traceback.format_exc()
    else:
        pytest.fail("invalid input was accepted")
