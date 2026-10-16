# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Check the fixed envelope shape before JSON constructs unbounded maps."""

from __future__ import annotations

import json
from typing import Any

from ._summary_errors import ModelUsageError
from .model_usage import RESERVED, is_public_id

_CHILDREN = {
    "root": {
        "counters": "counters",
        "sketches": "sketches",
        "model_usage": "extension",
    },
    "extension": {"models": "models"},
    "row": {
        "fields": "fields",
        "complete": "cohort",
        "histogram": "histogram",
        "excluded": "excluded",
        "provenance": "provenance",
    },
    "bin": {"reported": "reported", "complete": "cohort"},
}
_LIMITS = {
    "root": 15,
    "counters": 128,
    "sketches": 16,
    "payload": 2,
    "extension": 4,
    "models": 5004,
    "row": 7,
    "fields": 4,
    "field": 6,
    "cohort": 7,
    "bin": 2,
    "reported": 2,
    "excluded": 3,
    "provenance": 2,
    "origin": 4,
}
_SHAPES = {
    "root": {
        "accounting_id",
        "counters",
        "emitted_at_unix_nano",
        "epoch",
        "key_id",
        "observed_end_unix_nano",
        "observed_start_unix_nano",
        "producer_id",
        "scope_id",
        "sequence",
        "sketches",
        "version",
        "window_duration_unix_nano",
        "window_start_unix_nano",
    },
    "extension": {"contract_id", "histogram_id", "models", "version"},
    "row": {
        "cache_partition_violations",
        "complete",
        "excluded",
        "fields",
        "histogram",
        "provenance",
        "requests",
    },
    "fields": {"input", "output", "cache_read", "cache_write"},
    "field": {
        "tokens",
        "reported",
        "missing",
        "invalid",
        "conflict",
        "subset_violation",
    },
    "cohort": {
        "requests",
        "input_tokens",
        "output_tokens",
        "cache_read_input_tokens",
        "cache_write_input_tokens",
        "input_min",
        "input_max",
    },
    "bin": {"reported", "complete"},
    "reported": {"requests", "input_tokens"},
    "excluded": {"invalid_usage", "missing_split", "zero_origin_unknown"},
    "provenance": {"input", "output"},
    "origin": {"provider_reported", "inferred", "unavailable", "unknown"},
    "payload": {"data", "kind"},
}


def check_counts(data: bytes) -> bool:
    text = data.decode("utf-8")

    def reject_constant(value: str) -> Any:
        raise ValueError("invalid summary JSON")

    decoder = json.JSONDecoder(parse_constant=reject_constant)
    pos = 0
    present = False
    shape_error = False

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
        if isinstance(value, str):
            value.encode("utf-8")
        return value

    def object_(kind: str) -> None:
        nonlocal pos, present, shape_error
        whitespace()
        if pos == len(text) or text[pos] != "{":
            raise ValueError("invalid summary JSON")
        pos += 1
        count = real = 0
        seen: set[str] = set()
        whitespace()
        while pos < len(text) and text[pos] != "}":
            if count >= _LIMITS[kind]:
                raise ModelUsageError(
                    "model_count" if kind == "models" else "structure"
                )
            if count:
                if text[pos] != ",":
                    raise ValueError("invalid summary JSON")
                pos += 1
            key = scalar()
            whitespace()
            if (
                not isinstance(key, str)
                or key in seen
                or pos == len(text)
                or text[pos] != ":"
            ):
                raise ValueError("invalid summary JSON")
            seen.add(key)
            pos += 1
            if kind == "models":
                real += key not in RESERVED
                if real > 5000:
                    raise ModelUsageError("model_count")
                if key not in RESERVED and not is_public_id(key):
                    raise ModelUsageError("structure")
            if kind == "root" and key == "model_usage":
                present = True
            child = _CHILDREN.get(kind, {}).get(key)
            if kind in ("models", "fields", "provenance", "sketches"):
                child = {
                    "models": "row",
                    "fields": "field",
                    "provenance": "origin",
                    "sketches": "payload",
                }[kind]
            if child == "histogram":
                whitespace()
                if pos == len(text) or text[pos] != "[":
                    raise ModelUsageError("structure")
                pos += 1
                for n in range(4):
                    whitespace()
                    if n:
                        if pos == len(text) or text[pos] != ",":
                            raise ModelUsageError("structure")
                        pos += 1
                    object_("bin")
                whitespace()
                if pos == len(text) or text[pos] != "]":
                    raise ModelUsageError("structure")
                pos += 1
            elif child:
                object_(child)
            else:
                scalar()
            count += 1
            whitespace()
        if pos == len(text):
            raise ValueError("invalid summary JSON")
        pos += 1
        if kind in _SHAPES:
            required = _SHAPES[kind]
            if kind == "root" and "model_usage" in seen:
                required = required | {"model_usage"}
            shape_error = shape_error or seen != required

    object_("root")
    if pos != len(text) or shape_error:
        raise ModelUsageError("structure")
    return present
