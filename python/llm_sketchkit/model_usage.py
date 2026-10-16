# SPDX-License-Identifier: Apache-2.0
"""Model Usage v1 scalar composition; no normalization, routing LRU, or pricing.

Collection consumes the base producer's already accepted, alias-resolved values.
Retain returned blocks on global windows, not on evictable metric slices.
"""

from __future__ import annotations

import copy
import hashlib
import json
import re
from dataclasses import dataclass
from typing import Any

from ._summary_errors import ModelUsageError

MAX = (1 << 63) - 1
MAX_MODELS = 5000
HISTOGRAM_ID = "input-tiers-v1"
RESERVED = frozenset(("__overflow__", "__missing__", "__too_long__", "__unmapped__"))
EDGES = (100000, 200000, 272000, MAX)
FIELDS = ("input", "output", "cache_read", "cache_write")
TOTALS = (
    "input_tokens",
    "output_tokens",
    "cache_read_input_tokens",
    "cache_write_input_tokens",
)
ORIGINS = ("provider_reported", "inferred", "unavailable", "unknown")
FLAGS = ("invalid", "conflict", "subset_violation")
_ID = re.compile(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}", re.ASCII)
_CONTRACT = re.compile(r"model-usage-v1:[0-9a-f]{64}", re.ASCII)
_COHORT = {"requests", *TOTALS, "input_min", "input_max"}


def _require(ok: bool, reason: str = "conservation") -> None:
    if not ok:
        raise ModelUsageError(reason)


def _object(value: Any, keys: set[str]) -> None:
    _require(type(value) is dict and value.keys() == keys, "structure")


def _integer(value: Any) -> None:
    _require(type(value) is int and 0 <= value <= MAX, "range")


def is_public_id(value: Any) -> bool:
    return isinstance(value, str) and _ID.fullmatch(value) is not None


def check_names(models: Any, limit: int = MAX_MODELS) -> None:
    _require(type(limit) is int and 0 < limit <= MAX_MODELS, "range")
    _require(type(models) is dict, "structure")
    _require(len(models) <= MAX_MODELS + len(RESERVED), "model_count")
    _require(sum(k not in RESERVED for k in models) <= limit, "model_count")
    _require(all(k in RESERVED or is_public_id(k) for k in models), "structure")


def _cohort(c: Any) -> None:
    _object(c, _COHORT)
    for k in ("requests", *TOTALS):
        _integer(c[k])
    for k in ("input_min", "input_max"):
        if c[k] is not None:
            _integer(c[k])
    n, lo, hi, total = c["requests"], c["input_min"], c["input_max"], c["input_tokens"]
    if n == 0:
        _require(lo is None and hi is None and all(c[k] == 0 for k in TOTALS))
    else:
        _require(lo is not None and hi is not None)
        _require(lo <= hi and n * lo <= total <= n * hi)
        _require((n - 1) * lo + hi <= total <= lo + (n - 1) * hi)
    _require(c["cache_read_input_tokens"] + c["cache_write_input_tokens"] <= total)


def validate_row(row: Any) -> None:
    _object(
        row,
        {
            "requests",
            "fields",
            "complete",
            "histogram",
            "excluded",
            "provenance",
            "cache_partition_violations",
        },
    )
    _integer(row["requests"])
    _integer(row["cache_partition_violations"])
    n = row["requests"]
    _require(n > 0 and row["cache_partition_violations"] <= n)
    fields = row["fields"]
    _object(fields, set(FIELDS))
    for name, f in fields.items():
        _object(f, {"tokens", "reported", "missing", *FLAGS})
        for v in f.values():
            _integer(v)
        _require(f["reported"] + f["missing"] == n)
        _require(all(f[k] <= n for k in FLAGS))
        _require(f["reported"] > 0 or f["tokens"] == 0)
        _require(name not in ("input", "output") or f["subset_violation"] == 0)
    _object(row["excluded"], {"invalid_usage", "missing_split", "zero_origin_unknown"})
    for v in row["excluded"].values():
        _integer(v)
    c = row["complete"]
    _cohort(c)
    _require(c["requests"] + sum(row["excluded"].values()) == n)
    _require(row["excluded"]["invalid_usage"] >= row["cache_partition_violations"])
    for name, total in zip(FIELDS, TOTALS, strict=True):
        _require(c["requests"] <= fields[name]["reported"])
        _require(c[total] <= fields[name]["tokens"])
        if c["requests"] == fields[name]["reported"]:
            _require(c[total] == fields[name]["tokens"])
    _object(row["provenance"], {"input", "output"})
    for name, p in row["provenance"].items():
        _object(p, set(ORIGINS))
        for v in p.values():
            _integer(v)
        _require(sum(p.values()) == n and p["unavailable"] <= fields[name]["missing"])
        _require(p["provider_reported"] + p["inferred"] <= fields[name]["reported"])
    bins = row["histogram"]
    _require(type(bins) is list and len(bins) == 4, "structure")
    low = 0
    for b, high in zip(bins, EDGES, strict=True):
        _object(b, {"reported", "complete"})
        r, bc = b["reported"], b["complete"]
        _object(r, {"requests", "input_tokens"})
        for v in r.values():
            _integer(v)
        _cohort(bc)
        _require(r["requests"] * low <= r["input_tokens"] <= r["requests"] * high)
        _require(
            bc["requests"] <= r["requests"] and bc["input_tokens"] <= r["input_tokens"]
        )
        if bc["requests"] == r["requests"]:
            _require(bc["input_tokens"] == r["input_tokens"])
        if bc["requests"]:
            _require(low <= bc["input_min"] <= bc["input_max"] <= high)
        low = high + 1
    _require(
        sum(b["reported"]["requests"] for b in bins) == fields["input"]["reported"]
    )
    _require(
        sum(b["reported"]["input_tokens"] for b in bins) == fields["input"]["tokens"]
    )
    for key in ("requests", *TOTALS):
        _require(sum(b["complete"][key] for b in bins) == c[key])
    occupied = [b["complete"] for b in bins if b["complete"]["requests"]]
    _require(
        c["input_min"] == (min(b["input_min"] for b in occupied) if occupied else None)
    )
    _require(
        c["input_max"] == (max(b["input_max"] for b in occupied) if occupied else None)
    )


def validate(
    block: Any,
    counters: dict[str, int] | None = None,
    instantaneous: bool = False,
    max_models: int = MAX_MODELS,
) -> None:
    _object(block, {"contract_id", "histogram_id", "models", "version"})
    check_names(block["models"], max_models)
    _integer(block["version"])
    _require(isinstance(block["histogram_id"], str), "structure")
    _require(
        block["version"] == 1 and block["histogram_id"] == HISTOGRAM_ID,
        "unsupported_extension",
    )
    _require(
        isinstance(block["contract_id"], str)
        and _CONTRACT.fullmatch(block["contract_id"]) is not None,
        "structure",
    )
    _require(not instantaneous or not block["models"], "interval")
    for row in block["models"].values():
        validate_row(row)
    _require(all(v <= MAX for v in _reported_totals(block)))
    if counters is not None:
        keys = ("requests", *TOTALS)
        _require(all(k in counters for k in keys), "structure")
        _require(
            sum(r["requests"] for r in block["models"].values()) == counters["requests"]
        )
        for field, total in zip(FIELDS, TOTALS, strict=True):
            _require(
                sum(r["fields"][field]["tokens"] for r in block["models"].values())
                == counters[total]
            )


def compatible(a: dict[str, Any] | None, b: dict[str, Any] | None) -> None:
    _require((a is None) == (b is None), "presence")
    if a is not None and b is not None:
        _require(
            all(a[k] == b[k] for k in ("version", "histogram_id", "contract_id")),
            "contract",
        )


def _regression(a: Any, b: Any, key: str = "") -> None:
    if type(a) is dict:
        _require(type(b) is dict and a.keys() <= b.keys(), "regression")
        for k, v in a.items():
            _regression(v, b[k], k)
    elif type(a) is list:
        for x, y in zip(a, b, strict=True):
            _regression(x, y)
    elif key in ("input_min", "input_max"):
        if a is not None:
            _require(
                b is not None and (b <= a if key == "input_min" else b >= a),
                "regression",
            )
    else:
        _require(b >= a, "regression")


def check_regression(a: dict[str, Any] | None, b: dict[str, Any] | None) -> None:
    if a is not None and b is not None:
        _regression(a["models"], b["models"])


def _sum(a: Any, b: Any, key: str = "") -> Any:
    if type(a) is dict:
        return {k: _sum(v, b[k], k) for k, v in a.items()}
    if type(a) is list:
        return [_sum(x, y) for x, y in zip(a, b, strict=True)]
    if key in ("input_min", "input_max"):
        values = [v for v in (a, b) if v is not None]
        return (min(values) if key == "input_min" else max(values)) if values else None
    _require(a + b <= MAX, "overflow")
    return a + b


def _reported_totals(block: dict[str, Any]) -> list[int]:
    return [
        sum(row["requests"] for row in block["models"].values()),
        *(
            sum(row["fields"][k]["tokens"] for row in block["models"].values())
            for k in FIELDS
        ),
    ]


def merge(a: dict[str, Any] | None, b: dict[str, Any]) -> dict[str, Any]:
    """Validate and combine disjoint owners into independent scalar state."""
    validate(b)
    if a is None:
        return copy.deepcopy(b)
    validate(a)
    compatible(a, b)
    names = a["models"].keys() | b["models"].keys()
    check_names(dict.fromkeys(names))
    result = {k: v for k, v in a.items() if k != "models"}
    result["models"] = {
        k: _sum(a["models"][k], b["models"][k])
        if k in a["models"] and k in b["models"]
        else copy.deepcopy((a["models"] if k in a["models"] else b["models"])[k])
        for k in names
    }
    _require(all(v <= MAX for v in _reported_totals(result)), "overflow")
    return result


def canonical_config(value: Any) -> bytes:
    """Go encoding/json-compatible UTF-8 preimage, without Unicode normalization."""
    try:
        text = json.dumps(
            value,
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
            allow_nan=False,
        )
        for c in ("<", ">", "&", "\u2028", "\u2029"):
            text = text.replace(c, f"\\u{ord(c):04x}")
        return text.encode("utf-8")
    except (ValueError, TypeError, UnicodeError):
        raise ModelUsageError("structure") from None


def contract_id(
    accounting_id: str, model_map: dict[str, str], routing: dict[str, Any]
) -> str:
    """Hash resolved producer settings. No normalization/default migration."""
    _require(
        isinstance(accounting_id, str)
        and re.fullmatch(r"[A-Za-z0-9._:-]{1,128}", accounting_id) is not None,
        "structure",
    )
    _require(
        type(model_map) is dict
        and all(isinstance(k, str) and is_public_id(v) for k, v in model_map.items()),
        "structure",
    )
    _object(
        routing,
        {"algorithm", "max_label_bytes", "max_slices", "selected_slice", "slices"},
    )
    _require(
        routing["algorithm"] == "inactive-lru-v1"
        and type(routing["max_label_bytes"]) is int
        and routing["max_label_bytes"] == 256,
        "structure",
    )
    _require(
        type(routing["max_slices"]) is int and 0 < routing["max_slices"] <= MAX_MODELS,
        "range",
    )
    _require(type(routing["slices"]) is list, "structure")
    resolved = copy.deepcopy(routing)
    names: set[str] = set()
    selected = False
    for s in resolved["slices"]:
        _object(s, {"name", "keys", "from_resource_attributes"})
        _require(isinstance(s["name"], str) and s["name"] not in names, "structure")
        names.add(s["name"])
        for k in ("keys", "from_resource_attributes"):
            _require(
                type(s[k]) is list and all(isinstance(v, str) for v in s[k]),
                "structure",
            )
        if not s["from_resource_attributes"]:
            s["from_resource_attributes"] = s["keys"][:]
        if s["name"] == routing["selected_slice"]:
            _require(s["keys"] == ["gen_ai.request.model"], "structure")
            selected = True
    _require(selected, "structure")
    contract = {
        "accounting_id": accounting_id,
        "cohort_rule": "complete-split-v1",
        "histogram_id": HISTOGRAM_ID,
        "version": 1,
        "model_mapping_sha256": hashlib.sha256(canonical_config(model_map)).hexdigest(),
        "routing_sha256": hashlib.sha256(canonical_config(resolved)).hexdigest(),
    }
    return "model-usage-v1:" + hashlib.sha256(canonical_config(contract)).hexdigest()


def empty(contract: str) -> dict[str, Any]:
    block = {
        "contract_id": contract,
        "histogram_id": HISTOGRAM_ID,
        "models": {},
        "version": 1,
    }
    validate(block)
    return block


def classify_model(
    observed: str, model_map: dict[str, str], classification: str = ""
) -> str:
    """Preserve the existing router/classifier decision; never infer from labels."""
    _require(
        classification in ("", "__overflow__", "__missing__", "__too_long__"),
        "structure",
    )
    if classification:
        return classification
    result = model_map.get(observed, "__unmapped__")
    _require(result == "__unmapped__" or is_public_id(result), "structure")
    return result


@dataclass(frozen=True)
class Observation:
    value: int | None = None
    invalid: bool = False
    conflict: bool = False
    subset_violation: bool = False


def _empty_cohort() -> dict[str, Any]:
    return {
        "requests": 0,
        **dict.fromkeys(TOTALS, 0),
        "input_min": None,
        "input_max": None,
    }


def collect(
    block: dict[str, Any],
    model_id: str,
    fields: dict[str, Observation],
    input_origin: str = "unknown",
    output_origin: str = "unknown",
    max_models: int = MAX_MODELS,
) -> dict[str, Any]:
    """Stage one accepted attempt. Caller commits this and base updates together.

    All failures leave inputs unchanged. The source owns alias resolution and
    suppression of explicitly unavailable observations. No failed-span filtering.
    """
    validate(block, max_models=max_models)
    _require(model_id in RESERVED or is_public_id(model_id), "structure")
    _object(fields, set(FIELDS))
    _require(input_origin in ORIGINS and output_origin in ORIGINS, "structure")
    row: dict[str, Any] = {
        "requests": 1,
        "cache_partition_violations": 0,
        "fields": {},
        "complete": _empty_cohort(),
        "excluded": dict.fromkeys(
            ("invalid_usage", "missing_split", "zero_origin_unknown"), 0
        ),
        "histogram": [
            {
                "reported": {"requests": 0, "input_tokens": 0},
                "complete": _empty_cohort(),
            }
            for _ in EDGES
        ],
        "provenance": {
            name: {k: int(k == origin) for k in ORIGINS}
            for name, origin in (("input", input_origin), ("output", output_origin))
        },
    }
    for name, observation in fields.items():
        _require(isinstance(observation, Observation), "structure")
        if observation.value is not None:
            _integer(observation.value)
        _require(all(type(getattr(observation, k)) is bool for k in FLAGS), "structure")
        row["fields"][name] = {
            "tokens": observation.value or 0,
            "reported": int(observation.value is not None),
            "missing": int(observation.value is None),
            **{k: int(getattr(observation, k)) for k in FLAGS},
        }
    i, o, r, w = (fields[k].value for k in FIELDS)
    if i is not None and r is not None and w is not None:
        row["cache_partition_violations"] = int(r + w > i)
    index = next(
        (n for n, edge in enumerate(EDGES) if i is not None and i <= edge), None
    )
    if index is not None:
        row["histogram"][index]["reported"] = {"requests": 1, "input_tokens": i}
    if row["cache_partition_violations"] or any(
        getattr(f, k) for f in fields.values() for k in FLAGS
    ):
        row["excluded"]["invalid_usage"] = 1
    elif any(f.value is None for f in fields.values()):
        row["excluded"]["missing_split"] = 1
    elif i == o == 0 and "unknown" in (input_origin, output_origin):
        row["excluded"]["zero_origin_unknown"] = 1
    else:
        c = {
            "requests": 1,
            **dict(zip(TOTALS, (i, o, r, w), strict=True)),
            "input_min": i,
            "input_max": i,
        }
        row["complete"] = c
        row["histogram"][index]["complete"] = copy.deepcopy(c)
    validate_row(row)
    addition = {**block, "models": {model_id: row}}
    result = merge(block, addition)
    validate(result, max_models=max_models)
    return result
