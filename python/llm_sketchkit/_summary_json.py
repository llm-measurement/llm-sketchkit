# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Check the fixed envelope shape before JSON constructs unbounded maps."""

from __future__ import annotations

import json
from typing import Any


def check_counts(data: bytes) -> None:
    text = data.decode("utf-8")
    decoder = json.JSONDecoder()
    pos = 0

    def whitespace() -> None:
        nonlocal pos
        while pos < len(text) and text[pos] in " \t\r\n":
            pos += 1

    def scalar() -> Any:
        nonlocal pos
        whitespace()
        if pos == len(text) or text[pos] in "{[":
            raise ValueError("invalid summary JSON")
        value, pos = decoder.raw_decode(text, pos)
        return value

    def object_(kind: str, limit: int) -> None:
        nonlocal pos
        whitespace()
        if pos == len(text) or text[pos] != "{":
            raise ValueError("invalid summary JSON")
        pos += 1
        count = 0
        whitespace()
        while pos < len(text) and text[pos] != "}":
            if count >= limit:
                raise ValueError("invalid summary payload count")
            if count:
                if text[pos] != ",":
                    raise ValueError("invalid summary JSON")
                pos += 1
            key = scalar()
            whitespace()
            if not isinstance(key, str) or pos == len(text) or text[pos] != ":":
                raise ValueError("invalid summary JSON")
            pos += 1
            if kind == "root" and key in ("counters", "sketches"):
                object_(key, 128 if key == "counters" else 16)
            elif kind == "sketches":
                object_("payload", 2)
            else:
                scalar()
            count += 1
            whitespace()
        if pos == len(text):
            raise ValueError("invalid summary JSON")
        pos += 1

    object_("root", 14)
