# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Compare deterministic Go and Python sketch state over generated workloads."""

from __future__ import annotations

import argparse
import json
import os
import random
import subprocess
from pathlib import Path
from typing import Any

from llm_sketchkit import (
    PROMPT_V1,
    bloom,
    canonicalize_text_v1,
    frequentitems,
    hash64,
    hllpp,
    minhash,
)
from llm_sketchkit.hash import Secret

ROOT = Path(__file__).resolve().parents[1]
SECRET_TEXT = "sketchkit-differential-secret-v1"
HEAVY_TEXTS = (
    "agent-alpha",
    " agent-alpha\r\n",
    "agent-beta",
    "caf\u00e9",
    "cafe\u0301",
    "\ttool-search\n",
    "retrieval-document",
)
MAX_CASES = 100
MAX_UPDATES = 2_000


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--seed", type=int, default=20260903)
    parser.add_argument("--cases", type=int, default=12)
    parser.add_argument("--max-updates", type=int, default=320)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if args.cases < 1 or args.cases > MAX_CASES:
        raise SystemExit(f"--cases must be between 1 and {MAX_CASES}")
    if args.max_updates < 1 or args.max_updates > MAX_UPDATES:
        raise SystemExit(f"--max-updates must be between 1 and {MAX_UPDATES}")

    request = generate_request(args.seed, args.cases, args.max_updates)
    go_results = evaluate_go(request)
    python_results = evaluate_python(request)
    if go_results != python_results:
        explain_first_difference(go_results, python_results)
        raise SystemExit("Go and Python produced different sketch state")

    print(
        "cross-language differential cases matched: "
        f"seed={args.seed} cases={args.cases} max_updates={args.max_updates}"
    )


def generate_request(seed: int, case_count: int, max_updates: int) -> dict[str, Any]:
    rng = random.Random(seed)
    cases: list[dict[str, Any]] = []
    minimum_updates = max(1, max_updates // 2)
    for case_index in range(case_count):
        partition_count = 2 + case_index % 3
        partitions: list[list[dict[str, Any]]] = [
            [] for _ in range(partition_count)
        ]
        update_count = rng.randint(minimum_updates, max_updates)
        for update_index in range(update_count):
            if rng.random() < 0.72:
                text = rng.choice(HEAVY_TEXTS)
            else:
                text = (
                    f"unique-{case_index}-{update_index}-"
                    f"{rng.getrandbits(64):016x}"
                )
            partitions[update_index % partition_count].append(
                {"text": text, "weight": rng.randint(0, 100)}
            )
        cases.append(
            {
                "profile": "micro" if case_index % 2 == 0 else "small",
                "partitions": partitions,
            }
        )
    return {"cases": cases}


def evaluate_go(request: dict[str, Any]) -> list[dict[str, str]]:
    environment = os.environ.copy()
    environment["LLM_SKETCHKIT_DIFFERENTIAL_SECRET"] = SECRET_TEXT
    completed = subprocess.run(
        ["go", "run", "./internal/differential"],
        cwd=ROOT,
        env=environment,
        input=json.dumps(request, separators=(",", ":")),
        text=True,
        capture_output=True,
        check=False,
        timeout=300,
    )
    if completed.returncode != 0:
        raise RuntimeError(
            "Go differential helper failed: " + completed.stderr.strip()
        )
    payload = json.loads(completed.stdout)
    return list(payload["cases"])


def evaluate_python(request: dict[str, Any]) -> list[dict[str, str]]:
    secret = Secret(SECRET_TEXT.encode("utf-8"))
    results: list[dict[str, str]] = []
    for case in request["cases"]:
        profile = case["profile"]
        partitions = [
            build_python_partition(secret, profile, updates)
            for updates in case["partitions"]
        ]
        merged = tuple(sketch.clone() for sketch in partitions[0])
        for partition in partitions[1:]:
            for destination, source in zip(merged, partition, strict=True):
                destination.merge(source)
        results.append(
            {
                "hllpp_hex": merged[0].marshal_binary().hex(),
                "frequent_items_hex": merged[1].marshal_binary().hex(),
                "bloom_hex": merged[2].marshal_binary().hex(),
                "minhash_hex": merged[3].marshal_binary().hex(),
            }
        )
    return results


def build_python_partition(
    secret: Secret,
    profile: str,
    updates: list[dict[str, Any]],
) -> tuple[hllpp.Sketch, frequentitems.Sketch, bloom.Sketch, minhash.Sketch]:
    sketches = (
        hllpp.new(profile, PROMPT_V1),
        frequentitems.new(profile, PROMPT_V1),
        bloom.new(profile, PROMPT_V1),
        minhash.new(profile, PROMPT_V1),
    )
    for update in updates:
        digest = hash64(
            secret,
            PROMPT_V1,
            canonicalize_text_v1(str(update["text"])),
        )
        sketches[0].add_hash(digest)
        sketches[1].add_hash(digest, int(update["weight"]))
        sketches[2].add_hash(digest)
        sketches[3].add_hash(digest)
    return sketches


def explain_first_difference(
    go_results: list[dict[str, str]],
    python_results: list[dict[str, str]],
) -> None:
    for case_index, (go_case, python_case) in enumerate(
        zip(go_results, python_results, strict=False)
    ):
        for sketch_name in sorted(set(go_case) | set(python_case)):
            if go_case.get(sketch_name) != python_case.get(sketch_name):
                print(
                    f"first mismatch: case={case_index} sketch={sketch_name}",
                    flush=True,
                )
                return
    print(
        "result lengths differ: "
        f"go={len(go_results)} python={len(python_results)}",
        flush=True,
    )


if __name__ == "__main__":
    main()
