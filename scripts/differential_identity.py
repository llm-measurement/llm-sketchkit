# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Check Unicode 15 identities and HLL++ estimates against the current Go oracle."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import random
import struct
import subprocess
import sys
import tempfile
import unicodedata
from pathlib import Path
from typing import Any

from llm_sketchkit import canon, hash, hllpp, profiles

ROOT = Path(__file__).resolve().parents[1]
SECRET = "sketchkit-identity-vector-secret-v1"
MASK64 = (1 << 64) - 1


def estimate_case(case: dict[str, Any]) -> hllpp.Sketch:
    sketch = hllpp.new(case["profile"], profiles.PROMPT_V1)
    state = int(case["seed"])
    for _ in range(case["count"]):
        state = (state + 0x9E3779B97F4A7C15) & MASK64
        value = state
        value = ((value ^ (value >> 30)) * 0xBF58476D1CE4E5B9) & MASK64
        value = ((value ^ (value >> 27)) * 0x94D049BB133111EB) & MASK64
        sketch.add_hash(value ^ (value >> 31))
    if case["force_dense"]:
        sketch.force_dense()
    return sketch


def estimate_bits(value: float) -> str:
    return struct.pack(">d", value).hex()


def assert_estimate(actual: float, expected_bits: str, max_ulps: int) -> None:
    assert math.isfinite(actual) and actual >= 0
    distance = abs(int(estimate_bits(actual), 16) - int(expected_bits, 16))
    assert distance <= max_ulps, (estimate_bits(actual), expected_bits, distance)


def estimate_vectors() -> list[dict[str, Any]]:
    cases: list[dict[str, Any]] = []
    for path in sorted((ROOT / "vectors/identity").glob("hllpp_*.json")):
        payload = json.loads(path.read_text(encoding="utf-8"))
        assert payload["schema_version"] == 1
        cases.extend(payload["cases"])
    assert cases, "missing HLL++ estimate vectors"
    return cases


def unicode_cases(rows: list[list[int]], all_assigned: bool) -> list[str]:
    texts: list[str] = []
    sensitive: list[str] = []
    for codepoint, ccc, backward, decomposes in rows:
        char = chr(codepoint)
        assert canon._combining(char) == ccc, f"CCC mismatch U+{codepoint:04X}"
        if unicodedata.normalize("NFKD", char) == char:
            assert bool(backward) == (char in canon._BACKWARD_COMBINING)
        if ccc or backward or decomposes:
            sensitive.append(char)
            texts.extend(
                (char, "\u0300" * 29 + char + "\u0315" * 2, char + "\u0300" * 31)
            )
        elif all_assigned:
            texts.append(char)
    rng = random.Random(20261009)
    marks = [chr(c) for c, ccc, backward, _ in rows if ccc or backward]
    for _ in range(1000):
        texts.append(
            rng.choice(sensitive)
            + "".join(rng.choice(marks) for _ in range(rng.randint(28, 65)))
        )
    # Exercise all new CCC values around reordering, composition and blocking.
    for char in canon._CCC_15:
        for base in ("", "a", "\u00e1", "\u1100\u1161", "\u09c7"):
            for marks_text in ("\u0301", "\u0323\u0301", "\u0315\u0300"):
                texts.extend((base + char + marks_text, base + marks_text + char))
    return texts


def go_result(binary: Path, texts: list[str], cases: list[dict[str, Any]]) -> Any:
    env = os.environ.copy()
    env["LLM_SKETCHKIT_IDENTITY_SECRET"] = SECRET
    result = subprocess.run(
        [str(binary)],
        input=json.dumps({"texts": texts, "estimates": cases}),
        text=True,
        capture_output=True,
        check=True,
        env=env,
        timeout=300,
    )
    return json.loads(result.stdout)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--all-assigned",
        action="store_true",
        help="also check every inert assigned Unicode 15 scalar",
    )
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="sketchkit-identity-") as directory:
        binary = Path(directory) / "identity"
        subprocess.run(
            ["go", "build", "-o", str(binary), "./internal/identity"],
            cwd=ROOT,
            check=True,
            timeout=300,
        )
        rows = json.loads(
            subprocess.check_output(
                [str(binary), "--properties"],
                text=True,
                timeout=60,
            )
        )
        texts = unicode_cases(rows, args.all_assigned)
        fixture = json.loads(
            (ROOT / "vectors/identity/text_v1_unicode15.json").read_text(
                encoding="utf-8"
            )
        )
        texts.extend(case["input"] for case in fixture["cases"])
        secret = hash.Secret(SECRET.encode())
        for offset in range(0, len(texts), 2000):
            batch = texts[offset : offset + 2000]
            results = go_result(binary, batch, [])["texts"]
            assert len(results) == len(batch)
            for index, (text, expected) in enumerate(zip(batch, results, strict=True)):
                canonical = canon.canonicalize_text_v1(text)
                actual = {
                    "canonical_hex": canonical.hex(),
                    "digest_hex": hash.digest64_hex(
                        secret, profiles.PROMPT_V1, canonical
                    ),
                }
                if actual != expected:
                    raise AssertionError(
                        f"identity mismatch case={offset + index} input={ascii(text)} "
                        f"python={actual} go={expected}"
                    )
                assert canon.canonicalize_text_v1(canonical) == canonical
        estimates = estimate_vectors()
        results = go_result(binary, [], estimates)["estimates"]
        assert len(results) == len(estimates)
        for case, expected in zip(estimates, results, strict=True):
            sketch = estimate_case(case)
            assert sketch.marshal_binary().hex() == expected["wire_hex"], case["name"]
            assert (
                hashlib.sha256(sketch.marshal_binary()).hexdigest()
                == case["wire_sha256"]
            )
            assert_estimate(
                sketch.estimate(), expected["estimate_bits"], case["max_ulps"]
            )
            assert_estimate(sketch.estimate(), case["estimate_bits"], case["max_ulps"])
            assert_estimate(
                struct.unpack(">d", bytes.fromhex(expected["estimate_bits"]))[0],
                case["estimate_bits"],
                case["max_ulps"],
            )
    print(
        f"identity parity passed: Python {sys.version.split()[0]}, "
        f"Unicode {unicodedata.unidata_version}, {len(texts)} texts, "
        f"{len(estimates)} HLL++ cases; Unicode 15 Go oracle"
    )


if __name__ == "__main__":
    main()
