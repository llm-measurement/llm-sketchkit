# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Compare decoder acceptance, error categories and canonical bytes with Go."""

from __future__ import annotations

import json
import os
import random
import subprocess
import tempfile
from pathlib import Path
from typing import Any

from llm_sketchkit import _proto, bloom, frequentitems, hllpp, minhash, summary

ROOT = Path(__file__).resolve().parents[1]
MODULES = {"hllpp": hllpp, "frequent_items": frequentitems,
           "bloom": bloom, "minhash": minhash}


def category(exc: ValueError) -> str:
    name = type(exc).__name__
    if name == "UnknownProfileError":
        return "profile"
    if name in (
        "InvalidPrecisionError", "PrecisionMismatchError", "InvalidMapSizeError",
        "InvalidShapeError", "InvalidSignatureLengthError",
    ):
        return "shape"
    if name == "IncompatibleMergeError":
        return "keying"
    return "wire"


def python_outcome(kind: str, data: bytes) -> dict[str, str]:
    try:
        sketch = (summary.Envelope.parse(data) if kind == "summary"
                  else MODULES[kind].parse(data))
        return {"category": "accepted", "hex": sketch.marshal_binary().hex()}
    except ValueError as exc:
        return {"category": "summary" if kind == "summary" else category(exc)}


def collect(value: Any, out: list[bytes]) -> None:
    if isinstance(value, dict):
        for key, item in value.items():
            if key in ("serialized_hex", "merged_serialized_hex"):
                out.append(bytes.fromhex(item))
            elif key == "source_serialized_hex":
                out.extend(bytes.fromhex(s) for s in item.values())
            else:
                collect(item, out)
    elif isinstance(value, list):
        for item in value:
            collect(item, out)


def cases() -> list[tuple[str, bytes]]:
    bases: list[bytes] = []
    for path in sorted((ROOT / "vectors/sketches").glob("*.json")):
        collect(json.loads(path.read_text()), bases)
    rows: list[tuple[str, bytes]] = []
    rng = random.Random(20261009)
    for base in sorted(set(bases)):
        mutations = [base, b"", base[:1], base[:-1]]
        for _ in range(32):
            if not base:
                break
            data = bytearray(base)
            index = rng.randrange(min(len(data), 400))
            data[index] ^= rng.choice((1, 2, 8, 128, 255))
            mutations.extend((bytes(data), base[:index]))
        for depth in (1, 100, 101):
            mutations.append(base + b"\xa3\x06" * depth + b"\xa4\x06" * depth)
        for data in mutations:
            rows.extend((kind, data) for kind in MODULES)
    for name in ("envelope.json", "combined.json"):
        base = (ROOT / "vectors/summaries" / name).read_bytes()
        rows.append(("summary", base))
        for _ in range(128):
            data = bytearray(base)
            data[rng.randrange(len(data))] ^= rng.randrange(1, 256)
            rows.append(("summary", bytes(data)))
    return rows + audit_cases()


def audit_cases() -> list[tuple[str, bytes]]:
    """Retain the original malformed-input corpus alongside vector mutations."""
    rows: list[tuple[str, bytes]] = []
    rng = random.Random(20261009)
    valid: dict[tuple[str, str], bytes] = {}
    for kind, module in MODULES.items():
        for profile in ("micro", "small"):
            state = module.new(profile)
            for index in range(5):
                value = rng.getrandbits(64)
                if kind == "frequent_items":
                    state.add_hash(value, index + 1)
                else:
                    state.add_hash(value)
            valid[kind, profile] = state.marshal_binary()
            rows.append((kind, state.marshal_binary()))
    for kind in MODULES:
        wire = valid[kind, "micro"]
        rows.extend(((kind, b""), (kind, b"\0" * ((4 << 20) + 1))))
        for suffix in (b"\0", b"\xff", b"\x80" * 11, b"\x83\x01"):
            rows.append((kind, wire + suffix))
        for depth in (1, 50, 99, 100, 101, 150, 1000, 10001):
            rows.append((kind, wire + b"\xa3\x06" * depth + b"\xa4\x06" * depth))
        rows.extend((kind, wire[:index]) for index in range(min(len(wire), 301)))
        for _ in range(1000):
            data = bytearray(wire)
            mode = rng.randrange(4)
            if mode == 0:
                for _ in range(rng.randint(1, 3)):
                    data[rng.randrange(len(data))] = rng.randrange(256)
            elif mode == 1:
                at = rng.randrange(len(data) + 1)
                data[at:at] = rng.randbytes(rng.randrange(1, 5))
            elif mode == 2:
                at = rng.randrange(len(data))
                del data[at:at + rng.randrange(1, 5)]
            else:
                data.extend(rng.randbytes(rng.randrange(1, 8)))
            rows.append((kind, bytes(data)))
        message = _proto.parse_sketch(wire)
        message.metadata.profile = "SENTINEL_UNTRUSTED_PROFILE\nFAKE_LOG_ENTRY"
        rows.append((kind, _proto.serialize(message)))
    base = (ROOT / "vectors/summaries/envelope.json").read_bytes()
    rows.extend(("summary", data) for data in (
        base, b" " * ((8 << 20) + 1), b"[" * 2000 + b"0" + b"]" * 2000,
        b"null", b"[]", base + b"\n", base + b"{}", b"\xff",
        base[:-1] + b',"version":1}',
    ))
    document = json.loads(base)
    for value in (2**63, -1, True, 1.0, None):
        data = json.dumps({**document, "sequence": value}, sort_keys=True,
                          separators=(",", ":")).encode()
        rows.append(("summary", data))
    for _ in range(1000):
        data = bytearray(base)
        for _ in range(rng.randint(1, 3)):
            data[rng.randrange(len(data))] = rng.randrange(256)
        rows.append(("summary", bytes(data)))
    return rows


def main() -> None:
    rows = cases()
    mismatches: list[str] = []
    path = ROOT / "vectors/validation/error_precedence.json"
    precedence = {
        (row["kind"], bytes.fromhex(row["serialized_hex"])):
        (row["go_category"], row["python_category"])
        for row in json.loads(path.read_bytes())["cases"]
    }
    seen: set[tuple[str, bytes]] = set()
    # Files avoid a pipe deadlock when a decoded Bloom bitset is large.
    with tempfile.TemporaryDirectory(prefix="sketchkit-parity-") as directory:
        root = Path(directory)
        command = ["go", "build", "-o", str(root / "decode"), "./internal/malformed"]
        subprocess.run(
            command, cwd=ROOT, check=True, env=dict(os.environ, GOWORK="off")
        )
        with (
            (root / "input").open("w+") as source,
            (root / "output").open("w+") as output,
        ):
            for kind, data in rows:
                source.write(json.dumps({"Kind": kind, "Hex": data.hex()}) + "\n")
            source.seek(0)
            subprocess.run(
                [str(root / "decode")], stdin=source, stdout=output, check=True
            )
            output.seek(0)
            pairs = zip(rows, output, strict=True)
            for index, ((kind, data), line) in enumerate(pairs):
                expected = python_outcome(kind, data)
                actual = json.loads(line)
                if (kind, data) in precedence:
                    wanted = precedence[kind, data]
                    assert "accepted" not in wanted
                    assert (actual["category"], expected["category"]) == wanted
                    seen.add((kind, data))
                    continue
                if actual != expected:
                    mismatches.append(
                        f"case {index}, {kind}: Go={actual['category']} "
                        f"Python={expected['category']}; input={data[:80].hex()}"
                    )
    if mismatches:
        raise AssertionError("\n".join(mismatches))
    assert seen == set(precedence), "error-precedence vectors were not exercised"
    print(f"Malformed-input parity: {len(rows)} outcomes checked; "
          f"{len(seen)} preserved mixed-error precedence cases")


if __name__ == "__main__":
    main()
