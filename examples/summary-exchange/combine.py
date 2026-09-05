# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
"""Combine exported files locally; no network access and no hashing secret needed."""

import argparse
import json
from dataclasses import asdict
from pathlib import Path

from llm_sketchkit import frequentitems, hllpp, summary


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expected", nargs="+", required=True)
    parser.add_argument("--window-start", type=int, required=True)
    parser.add_argument("files", nargs="+", type=Path)
    args = parser.parse_args()
    if len(args.files) > 1024:
        parser.error("at most 1024 files may be read")
    documents = []
    size = 0
    try:
        for path in args.files:
            with path.open("rb") as stream:
                data = stream.read(summary.MAX_BYTES + 1)
            size += len(data)
            if size > 64 << 20:
                parser.error("input exceeds 64 MiB")
            doc = summary.Envelope.parse(data)
            if doc.window_start_unix_nano == args.window_start:
                documents.append(doc)
        result = summary.combine(documents, args.expected)
        estimates = {
            name: hllpp.parse(payload.data).estimate()
            for name, payload in result.sketches.items() if payload.kind == "hllpp"
        }
        heavy_items = {
            name: [
                {**asdict(item), "hash": f"{item.hash:016x}"}
                for item in frequentitems.parse(payload.data).frequent_items(
                    frequentitems.NO_FALSE_NEGATIVES
                )[:20]
            ]
            for name, payload in result.sketches.items()
            if payload.kind == "frequent_items"
        }
    except (OSError, summary.SummaryError) as exc:
        parser.error(str(exc))
    print(json.dumps({
        "window_start_unix_nano": args.window_start,
        "counters": result.counters, "distinct_estimates": estimates,
        "heavy_items": heavy_items, "missing": result.missing,
        "partial": result.partial, "sources": [asdict(s) for s in result.sources],
    }, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
