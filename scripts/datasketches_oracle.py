#!/usr/bin/env python3
# Code authors: Vijay Erramilli and Codex
"""Generate Apache DataSketches frequent-items oracle comparison evidence."""

from __future__ import annotations

import argparse
import bisect
import copy
import importlib.metadata
import json
import math
import random
import sys
from pathlib import Path
from typing import Any, cast

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "python"))

from llm_sketchkit import frequentitems, hashfamily, profiles  # noqa: E402

DEFAULT_FIXTURE = ROOT / "vectors/oracles/datasketches_frequent_items.json"
DEFAULT_JSON = ROOT / "reports/datasketches_oracle.json"
DEFAULT_MARKDOWN = ROOT / "reports/datasketches_oracle.md"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixture", type=Path, default=DEFAULT_FIXTURE)
    parser.add_argument("--json", type=Path, default=DEFAULT_JSON)
    parser.add_argument("--markdown", type=Path, default=DEFAULT_MARKDOWN)
    parser.add_argument(
        "--check",
        action="store_true",
        help="fail if generated outputs differ from the checked-in files",
    )
    args = parser.parse_args()

    try:
        from datasketches import (  # type: ignore[import-untyped]
            frequent_items_error_type,
            frequent_items_sketch,
        )
    except ModuleNotFoundError as exc:
        raise SystemExit(
            "DataSketches oracle dependency missing. Install with: "
            "python -m pip install -e '.[oracle]'"
        ) from exc

    fixture = read_json(args.fixture)
    results = run_oracle(
        fixture,
        frequent_items_sketch=frequent_items_sketch,
        error_type=frequent_items_error_type,
    )
    assert_results(results)
    json_text = json.dumps(results, indent=2, sort_keys=True) + "\n"
    markdown_text = render_markdown(results)

    if args.check:
        check_file(args.json, json_text)
        check_file(args.markdown, markdown_text)
        return 0

    args.json.parent.mkdir(parents=True, exist_ok=True)
    args.markdown.parent.mkdir(parents=True, exist_ok=True)
    args.json.write_text(json_text, encoding="utf-8")
    args.markdown.write_text(markdown_text, encoding="utf-8")
    return 0


def run_oracle(
    fixture: dict[str, Any],
    *,
    frequent_items_sketch: Any,
    error_type: Any,
) -> dict[str, Any]:
    workloads = []
    for spec in fixture["workloads"]:
        workloads.append(
            run_workload(
                spec,
                frequent_items_sketch=frequent_items_sketch,
                error_type=error_type,
            )
        )

    return {
        "schema_version": 1,
        "oracle": fixture["oracle"],
        "datasketches_version": importlib.metadata.version("datasketches"),
        "scope": (
            "Weighted frequent-items query semantics only. HLL byte or estimate "
            "equality is deliberately out of scope because DataSketches HLL is a "
            "different algorithm family than llm-sketchkit HLL++."
        ),
        "workloads": workloads,
    }


def assert_results(results: dict[str, Any]) -> None:
    for workload in results["workloads"]:
        name = workload["name"]
        checks = workload["single_sketch"]["checks"]
        required_checks = {
            "sketchkit_brackets_true_top_k": checks["sketchkit_brackets_true_top_k"],
            "datasketches_brackets_true_top_k": checks[
                "datasketches_brackets_true_top_k"
            ],
            "sketchkit_no_false_positives_valid": checks[
                "sketchkit_no_false_positives_valid"
            ],
            "datasketches_no_false_positives_valid": checks[
                "datasketches_no_false_positives_valid"
            ],
        }
        for check_name, passed in required_checks.items():
            if not passed:
                raise SystemExit(f"{name}: oracle check failed: {check_name}")

        for check_name in [
            "sketchkit_no_false_negatives_top_k_recall",
            "datasketches_no_false_negatives_top_k_recall",
        ]:
            if checks[check_name] != 1.0:
                raise SystemExit(f"{name}: oracle check failed: {check_name}")

        for row in workload["partitioned_merge"]["orders"]:
            order = row["order"]
            for engine in ["sketchkit", "datasketches"]:
                summary = row[engine]
                if summary["top_k_recall"] != 1.0:
                    raise SystemExit(
                        f"{name}/{order}/{engine}: top-k recall was not 1.0"
                    )
                if not summary["no_false_positives_valid"]:
                    raise SystemExit(
                        f"{name}/{order}/{engine}: no-false-positive check failed"
                    )


def run_workload(
    spec: dict[str, Any],
    *,
    frequent_items_sketch: Any,
    error_type: Any,
) -> dict[str, Any]:
    updates = list(generate_updates(spec))
    exact = exact_counts(updates)
    top_k = int(spec["top_k"])
    threshold = top_items(exact, top_k)[-1][1] - 1

    sketchkit = build_sketchkit(spec["profile"], updates)
    datasketches = build_datasketches(
        int(spec["datasketches_lg_max_k"]),
        updates,
        frequent_items_sketch=frequent_items_sketch,
    )

    single = summarize_pair(
        exact,
        threshold,
        top_k,
        sketchkit=sketchkit,
        datasketches=datasketches,
        error_type=error_type,
    )
    merge = summarize_merges(
        spec,
        updates,
        exact,
        threshold,
        top_k,
        frequent_items_sketch=frequent_items_sketch,
        error_type=error_type,
    )

    map_size = profiles.FREQUENT_ITEMS_MAP_SIZES[str(spec["profile"])]
    ds_lg = int(spec["datasketches_lg_max_k"])
    return {
        "name": spec["name"],
        "kind": spec["kind"],
        "profile": spec["profile"],
        "updates": len(updates),
        "distinct_keys": len(exact),
        "total_weight": sum(weight for _, weight in updates),
        "top_k": top_k,
        "top_k_threshold_exclusive": threshold,
        "sketchkit_map_size": map_size,
        "datasketches_lg_max_k": ds_lg,
        "datasketches_nominal_capacity": int((1 << ds_lg) * 0.75),
        "single_sketch": single,
        "partitioned_merge": merge,
    }


def generate_updates(spec: dict[str, Any]) -> list[tuple[int, int]]:
    kind = spec["kind"]
    if kind == "zipf":
        return generate_zipf_updates(spec)
    if kind == "tail_churn":
        return generate_tail_churn_updates(spec)
    raise ValueError(f"unknown workload kind: {kind}")


def generate_zipf_updates(spec: dict[str, Any]) -> list[tuple[int, int]]:
    update_count = int(spec["updates"])
    key_count = int(spec["key_count"])
    alpha = float(spec["alpha"])
    rng = random.Random(int(spec["seed"]))
    weights = [1.0 / (rank**alpha) for rank in range(1, key_count + 1)]
    total = sum(weights)
    cumulative = []
    running = 0.0
    for weight in weights:
        running += weight / total
        cumulative.append(running)

    updates = []
    for i in range(update_count):
        rank = bisect.bisect_left(cumulative, rng.random()) + 1
        key = hashfamily.mix64(rank)
        weight = 1
        if (i * 17 + rank) % 7 == 0:
            weight += 1
        if (i + rank) % 113 == 0:
            weight += 3
        updates.append((key, weight))
    return updates


def generate_tail_churn_updates(spec: dict[str, Any]) -> list[tuple[int, int]]:
    update_count = int(spec["updates"])
    heavy_keys = int(spec["heavy_keys"])
    tail_keys = int(spec["tail_keys"])
    updates = []
    for i in range(update_count):
        if i % 5 == 0:
            rank = heavy_keys + 1 + ((i // 5) % tail_keys)
            weight = 1
        else:
            rank = 1 + ((i * 17) % heavy_keys)
            weight = 1 + int(i % 11 == 0)
        updates.append((hashfamily.mix64(rank), weight))
    return updates


def build_sketchkit(profile: str, updates: list[tuple[int, int]]) -> Any:
    sketch = frequentitems.new(profile)
    for key, weight in updates:
        sketch.add_hash(key, weight)
    return sketch


def build_datasketches(
    lg_max_k: int,
    updates: list[tuple[int, int]],
    *,
    frequent_items_sketch: Any,
) -> Any:
    sketch = frequent_items_sketch(lg_max_k)
    for key, weight in updates:
        sketch.update(key, weight)
    return sketch


def summarize_pair(
    exact: dict[int, int],
    threshold: int,
    top_k: int,
    *,
    sketchkit: Any,
    datasketches: Any,
    error_type: Any,
) -> dict[str, Any]:
    top = top_items(exact, top_k)
    top_keys = {key for key, _ in top}
    sk_nfn = sketchkit_candidates(sketchkit, threshold, no_false_negatives=True)
    sk_nfp = sketchkit_candidates(sketchkit, threshold, no_false_negatives=False)
    ds_nfn = datasketches_candidates(
        datasketches,
        error_type.NO_FALSE_NEGATIVES,
        threshold,
    )
    ds_nfp = datasketches_candidates(
        datasketches,
        error_type.NO_FALSE_POSITIVES,
        threshold,
    )

    return {
        "sketchkit": summarize_sketchkit(sketchkit, exact, threshold, top_keys),
        "datasketches": summarize_datasketches(
            datasketches,
            exact,
            threshold,
            top_keys,
            ds_nfn,
            ds_nfp,
        ),
        "top_items": top_item_rows(top, sketchkit, datasketches),
        "checks": {
            "sketchkit_brackets_true_top_k": brackets_sketchkit(
                sketchkit,
                exact,
                top_keys,
            ),
            "datasketches_brackets_true_top_k": brackets_datasketches(
                datasketches,
                exact,
                top_keys,
            ),
            "sketchkit_no_false_negatives_top_k_recall": recall(top_keys, sk_nfn),
            "datasketches_no_false_negatives_top_k_recall": recall(top_keys, ds_nfn),
            "sketchkit_no_false_positives_valid": no_false_positives_valid(
                exact,
                sk_nfp,
                threshold,
            ),
            "datasketches_no_false_positives_valid": no_false_positives_valid(
                exact,
                ds_nfp,
                threshold,
            ),
        },
    }


def summarize_merges(
    spec: dict[str, Any],
    updates: list[tuple[int, int]],
    exact: dict[int, int],
    threshold: int,
    top_k: int,
    *,
    frequent_items_sketch: Any,
    error_type: Any,
) -> dict[str, Any]:
    partitions = int(spec["partitions"])
    part_updates = partition_updates(updates, partitions)
    sk_parts = [build_sketchkit(str(spec["profile"]), part) for part in part_updates]
    ds_parts = [
        build_datasketches(
            int(spec["datasketches_lg_max_k"]),
            part,
            frequent_items_sketch=frequent_items_sketch,
        )
        for part in part_updates
    ]
    orders = merge_orders(partitions)
    rows = []
    top_keys = {key for key, _ in top_items(exact, top_k)}
    for name, order in orders.items():
        sk_merged = frequentitems.new(str(spec["profile"]))
        ds_merged = frequent_items_sketch(int(spec["datasketches_lg_max_k"]))
        for index in order:
            sk_merged.merge(sk_parts[index])
            ds_merged.merge(copy.copy(ds_parts[index]))
        rows.append(
            {
                "order": name,
                "sketchkit": summarize_sketchkit(
                    sk_merged,
                    exact,
                    threshold,
                    top_keys,
                ),
                "datasketches": summarize_datasketches(
                    ds_merged,
                    exact,
                    threshold,
                    top_keys,
                    datasketches_candidates(
                        ds_merged,
                        error_type.NO_FALSE_NEGATIVES,
                        threshold,
                    ),
                    datasketches_candidates(
                        ds_merged,
                        error_type.NO_FALSE_POSITIVES,
                        threshold,
                    ),
                ),
            }
        )
    return {"partitions": partitions, "orders": rows}


def summarize_sketchkit(
    sketch: Any,
    exact: dict[int, int],
    threshold: int,
    top_keys: set[int],
) -> dict[str, Any]:
    nfn = sketchkit_candidates(sketch, threshold, no_false_negatives=True)
    nfp = sketchkit_candidates(sketch, threshold, no_false_negatives=False)
    return {
        "tracked_items": len(sketch),
        "total_weight": sketch.total_weight(),
        "max_error": sketch.max_error(),
        "no_false_negatives_candidates": len(nfn),
        "no_false_positives_candidates": len(nfp),
        "top_k_recall": recall(top_keys, nfn),
        "no_false_positives_valid": no_false_positives_valid(exact, nfp, threshold),
    }


def summarize_datasketches(
    sketch: Any,
    exact: dict[int, int],
    threshold: int,
    top_keys: set[int],
    nfn: set[int],
    nfp: set[int],
) -> dict[str, Any]:
    return {
        "tracked_items": sketch.num_active_items,
        "total_weight": sketch.total_weight,
        "epsilon": sketch.epsilon,
        "apriori_error_estimate": math.ceil(sketch.epsilon * sketch.total_weight),
        "no_false_negatives_candidates": len(nfn),
        "no_false_positives_candidates": len(nfp),
        "top_k_recall": recall(top_keys, nfn),
        "no_false_positives_valid": no_false_positives_valid(exact, nfp, threshold),
    }


def top_item_rows(
    top: list[tuple[int, int]],
    sketchkit: Any,
    datasketches: Any,
) -> list[dict[str, Any]]:
    rows = []
    for rank, (key, true_weight) in enumerate(top, start=1):
        rows.append(
            {
                "rank": rank,
                "hash_hex": f"{key:016x}",
                "true_weight": true_weight,
                "sketchkit": {
                    "estimate": sketchkit.estimate_hash(key),
                    "lower_bound": sketchkit.lower_bound_hash(key),
                    "upper_bound": sketchkit.upper_bound_hash(key),
                },
                "datasketches": {
                    "estimate": datasketches.get_estimate(key),
                    "lower_bound": datasketches.get_lower_bound(key),
                    "upper_bound": datasketches.get_upper_bound(key),
                },
            }
        )
    return rows


def sketchkit_candidates(
    sketch: Any,
    threshold: int,
    *,
    no_false_negatives: bool,
) -> set[int]:
    if no_false_negatives:
        return {item.hash for item in sketch.items() if item.upper_bound > threshold}
    return {item.hash for item in sketch.items() if item.lower_bound > threshold}


def datasketches_candidates(
    sketch: Any,
    mode: Any,
    threshold: int,
) -> set[int]:
    return {int(row[0]) for row in sketch.get_frequent_items(mode, threshold)}


def brackets_sketchkit(sketch: Any, exact: dict[int, int], keys: set[int]) -> bool:
    return all(
        sketch.lower_bound_hash(key) <= exact[key] <= sketch.upper_bound_hash(key)
        for key in keys
    )


def brackets_datasketches(sketch: Any, exact: dict[int, int], keys: set[int]) -> bool:
    return all(
        sketch.get_lower_bound(key) <= exact[key] <= sketch.get_upper_bound(key)
        for key in keys
    )


def no_false_positives_valid(
    exact: dict[int, int],
    candidates: set[int],
    threshold: int,
) -> bool:
    return all(exact.get(key, 0) > threshold for key in candidates)


def recall(expected: set[int], actual: set[int]) -> float:
    if not expected:
        return 1.0
    return len(expected & actual) / len(expected)


def exact_counts(updates: list[tuple[int, int]]) -> dict[int, int]:
    counts: dict[int, int] = {}
    for key, weight in updates:
        counts[key] = counts.get(key, 0) + weight
    return counts


def top_items(exact: dict[int, int], limit: int) -> list[tuple[int, int]]:
    return sorted(exact.items(), key=lambda item: (-item[1], item[0]))[:limit]


def partition_updates(
    updates: list[tuple[int, int]],
    partitions: int,
) -> list[list[tuple[int, int]]]:
    out: list[list[tuple[int, int]]] = [[] for _ in range(partitions)]
    for index, update in enumerate(updates):
        out[index % partitions].append(update)
    return out


def merge_orders(partitions: int) -> dict[str, list[int]]:
    forward = list(range(partitions))
    reverse = list(reversed(forward))
    interleaved = forward[::2] + forward[1::2]
    return {"forward": forward, "reverse": reverse, "interleaved": interleaved}


def render_markdown(results: dict[str, Any]) -> str:
    lines = [
        "# DataSketches Oracle Comparison",
        "",
        "This report compares `llm-sketchkit` weighted frequent-items query",
        "semantics against Apache DataSketches Python. DataSketches is used as",
        "a reference oracle only; it is not a runtime dependency, wire-format",
        "authority, or HLL++ semantic authority for this project.",
        "",
        f"- DataSketches Python: `{results['datasketches_version']}`",
        "- Fixture: `vectors/oracles/datasketches_frequent_items.json`",
        "",
        "## Summary",
        "",
        "| Workload | Total weight | Distinct keys | Threshold | "
        "Sketchkit NFN recall | DataSketches NFN recall | "
        "Sketchkit NFP valid | DataSketches NFP valid |",
        "|---|---:|---:|---:|---:|---:|---|---|",
    ]
    for workload in results["workloads"]:
        checks = workload["single_sketch"]["checks"]
        lines.append(
            "| {name} | {total} | {distinct} | {threshold} | {sk_recall:.2f} | "
            "{ds_recall:.2f} | {sk_fp} | {ds_fp} |".format(
                name=workload["name"],
                total=workload["total_weight"],
                distinct=workload["distinct_keys"],
                threshold=workload["top_k_threshold_exclusive"],
                sk_recall=checks["sketchkit_no_false_negatives_top_k_recall"],
                ds_recall=checks["datasketches_no_false_negatives_top_k_recall"],
                sk_fp=yes_no(checks["sketchkit_no_false_positives_valid"]),
                ds_fp=yes_no(checks["datasketches_no_false_positives_valid"]),
            )
        )

    lines.extend(
        [
            "",
            "## Workload Details",
            "",
        ]
    )
    for workload in results["workloads"]:
        single = workload["single_sketch"]
        lines.extend(render_workload_markdown(workload, single))

    return "\n".join(lines).rstrip() + "\n"


def render_workload_markdown(
    workload: dict[str, Any],
    single: dict[str, Any],
) -> list[str]:
    sk = single["sketchkit"]
    ds = single["datasketches"]
    lines = [
        f"### `{workload['name']}`",
        "",
        f"- Profile: `{workload['profile']}`",
        f"- Updates: `{workload['updates']}`",
        f"- Distinct keys: `{workload['distinct_keys']}`",
        f"- Total weight: `{workload['total_weight']}`",
        f"- Top-{workload['top_k']} threshold: "
        f"`>{workload['top_k_threshold_exclusive']}`",
        f"- Sketchkit map size: `{workload['sketchkit_map_size']}`",
        "- DataSketches lg_max_k: "
        f"`{workload['datasketches_lg_max_k']}` "
        f"(nominal capacity `{workload['datasketches_nominal_capacity']}`)",
        "",
        "| Engine | Tracked items | Error signal | NFN candidates | "
        "NFP candidates | Top-k recall | NFP valid |",
        "|---|---:|---:|---:|---:|---:|---|",
        "| llm-sketchkit | {tracked} | {err} | {nfn} | {nfp} | "
        "{recall:.2f} | {fp} |".format(
            tracked=sk["tracked_items"],
            err=sk["max_error"],
            nfn=sk["no_false_negatives_candidates"],
            nfp=sk["no_false_positives_candidates"],
            recall=sk["top_k_recall"],
            fp=yes_no(sk["no_false_positives_valid"]),
        ),
        "| DataSketches | {tracked} | {err} | {nfn} | {nfp} | "
        "{recall:.2f} | {fp} |".format(
            tracked=ds["tracked_items"],
            err=ds["apriori_error_estimate"],
            nfn=ds["no_false_negatives_candidates"],
            nfp=ds["no_false_positives_candidates"],
            recall=ds["top_k_recall"],
            fp=yes_no(ds["no_false_positives_valid"]),
        ),
        "",
        "Partitioned merge orders all preserve top-k recall and no-false-positive",
        "validity in both engines. Full per-order rows and top-item intervals are",
        "in `reports/datasketches_oracle.json`.",
        "",
    ]
    return lines


def yes_no(value: bool) -> str:
    return "yes" if value else "no"


def read_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"{path} must contain a JSON object")
    return cast(dict[str, Any], value)


def check_file(path: Path, expected: str) -> None:
    actual = path.read_text(encoding="utf-8")
    if actual != expected:
        raise SystemExit(f"{path} is out of date; rerun scripts/datasketches_oracle.py")


if __name__ == "__main__":
    raise SystemExit(main())
