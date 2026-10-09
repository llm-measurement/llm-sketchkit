# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Generate new identity vector files from Go, refusing to overwrite any file."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import tempfile
from pathlib import Path
from typing import Any

from llm_sketchkit import canon, profiles

from scripts.differential_identity import ROOT, SECRET, go_result


def canonical_cases() -> list[tuple[str, str]]:
    cases = [
        (f"control_{codepoint:04x}", f" {chr(codepoint)}value{chr(codepoint)} ")
        for codepoint in range(0x1C, 0x20)
    ]
    cases.extend(
        [
            ("all_white_space", canon._WHITE_SPACE),
            ("white_space_edges", canon._WHITE_SPACE + "x" + canon._WHITE_SPACE),
            ("white_space_interior", "a" + canon._WHITE_SPACE + "b"),
            ("non_white_space_edges", "\u180e\u200b\u2060\ufeff"),
            ("newlines", "\r\nx\r\ny\rz\n"),
            ("precomposed_tail", "\u00e0" + "\u0300" * 30),
            ("compatibility_tail", "\u00a8" + "\u0300" * 30),
            ("compatibility_nonstarter", "\uff9e" * 31),
            ("double_decomposition", "\u0344" * 16),
            ("tibetan_decomposition", "\u0f73" * 16),
            ("hangul_tail", "\uac01" + "\u0300" * 29),
            ("jamo_backward", "\u1100" + "\u1161" * 31),
            ("bengali_backward", "\u09c7" + "\u09be" * 31),
            ("existing_cgj", "a" + "\u0300" * 30 + "\u034f\u0300"),
            ("starter_resets", "\u0300" * 30 + "a" + "\u0300" * 30),
            ("reorder_across_boundary", "a" + "\u0315" * 30 + "\u0300"),
            ("supplementary_decomposition", "\U0001d160" + "\u0300" * 30),
        ]
    )
    for count in (29, 30, 31, 60, 61, 91):
        cases.extend(
            [
                (f"leading_{count}", "\u0300" * count),
                (f"after_starter_{count}", "a" + "\u0300" * count),
            ]
        )
    for char in canon._CCC_15:
        cases.extend(
            [
                (f"unicode15_reorder_{ord(char):x}", "a\u0315" + char + "\u0301"),
                (f"unicode15_stream_{ord(char):x}", "a" + char * 31),
            ]
        )
    return cases


def estimate_cases() -> list[dict[str, Any]]:
    cases: list[dict[str, Any]] = []
    for profile, config in profiles.HLLPP_PROFILES.items():
        m = 1 << config.normal_precision
        for label, count, dense, ulps in (
            ("empty_sparse", 0, False, 0),
            ("empty_dense", 0, True, 0),
            ("singleton", 1, False, 2),
            ("sparse", 100, False, 2),
            ("promotion_before", config.promotion_threshold, False, 2),
            ("promotion_after", config.promotion_threshold + 64, False, 2),
            ("forced_dense", 100, True, 2),
            ("linear_counting", m // 2, False, 2),
            ("bias_m", m, False, 0),
            ("bias_2m", 2 * m, False, 0),
            ("bias_4m", 4 * m, False, 0),
            ("raw", 10 * m, False, 0),
        ):
            for seed in (1, 20261009):
                cases.append(
                    {
                        "name": f"{profile}_{label}_{seed}",
                        "profile": profile,
                        "count": count,
                        "seed": seed,
                        "force_dense": dense,
                        "max_ulps": ulps,
                    }
                )
    return cases


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    paths = [
        args.output_dir / name
        for name in (
            "text_v1_unicode15.json",
            "hllpp_estimates.json",
            "hllpp_bias_regressions.json",
        )
    ]
    if any(path.exists() for path in paths):
        raise SystemExit("Refusing to overwrite an existing vector file")
    with tempfile.TemporaryDirectory(prefix="sketchkit-vector-") as directory:
        binary = Path(directory) / "identity"
        subprocess.run(
            ["go", "build", "-o", str(binary), "./internal/identity"],
            cwd=ROOT,
            check=True,
            timeout=300,
        )
        # This checks both Unicode versions, even though the property output is unused.
        subprocess.run(
            [str(binary), "--properties"],
            stdout=subprocess.DEVNULL,
            check=True,
            timeout=60,
        )
        texts = canonical_cases()
        estimates = estimate_cases()
        grid_count = len(estimates)
        for profile, count in (("micro", 3263), ("small", 13481), ("default", 20007)):
            estimates.append(
                {
                    "name": f"{profile}_left_to_right_bias",
                    "profile": profile,
                    "count": count,
                    "seed": 1,
                    "force_dense": True,
                    "max_ulps": 0,
                }
            )
        result = go_result(binary, [text for _, text in texts], estimates)
    canonical_vectors = [
        dict(name=name, input=text, **expected)
        for (name, text), expected in zip(texts, result["texts"], strict=True)
    ]
    for case, expected in zip(estimates, result["estimates"], strict=True):
        case["estimate_bits"] = expected["estimate_bits"]
        case["wire_sha256"] = hashlib.sha256(
            bytes.fromhex(expected["wire_hex"])
        ).hexdigest()
    for path, payload in zip(
        paths,
        (
            {
                "schema_version": 1,
                "unicode_version": "15.0.0",
                "secret": SECRET,
                "cases": canonical_vectors,
            },
            {
                "schema_version": 1,
                "generator": "splitmix64",
                "cases": estimates[:grid_count],
            },
            {
                "schema_version": 1,
                "generator": "splitmix64",
                "cases": estimates[grid_count:],
            },
        ),
        strict=True,
    ):
        with path.open("x", encoding="utf-8") as output:
            output.write(json.dumps(payload, indent=2, ensure_ascii=True) + "\n")
        print(path)


if __name__ == "__main__":
    main()
