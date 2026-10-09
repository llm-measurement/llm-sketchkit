# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex

import json
from pathlib import Path

import pytest
from llm_sketchkit import bloom, frequentitems, hllpp, minhash, summary


def test_interval_vectors() -> None:
    root = Path(__file__).resolve().parents[1]
    vector_path = root / "vectors/validation/summary_intervals.json"
    vectors = json.loads(vector_path.read_text())
    for case in vectors["cases"]:
        docs = []
        for i in range(case["repeat"]):
            doc = summary.Envelope.parse(
                (root / "vectors/summaries/envelope.json").read_bytes()
            )
            doc.epoch = f'e{i}{case.get("suffix", "")}'
            doc.counters = {"requests": case["count"]}
            doc.observed_start_unix_nano = case["start"]
            doc.observed_end_unix_nano = case["end"]
            if not case["sketches"]:
                doc.sketches = {}
            docs.append(doc)
        if case.get("suffix"):
            other = summary.Envelope.parse(
                (root / "vectors/summaries/envelope.json").read_bytes()
            )
            other.epoch = "e0m"
            other.observed_start_unix_nano = 90
            other.sketches = {}
            docs.append(other)
        if case["error"]:
            with pytest.raises(summary.SummaryError):
                summary.combine(docs, [docs[0].producer_id])
        else:
            summary.combine(docs, [docs[0].producer_id])


@pytest.mark.parametrize("module", [hllpp, frequentitems, bloom, minhash])
@pytest.mark.parametrize("depth", [1, 100, 101, 10002])
def test_groups_rejected_at_all_depths(module, depth: int):  # type: ignore[no-untyped-def]
    wire = module.Sketch("micro").marshal_binary()
    with pytest.raises(module.InvalidWireEncodingError):
        module.parse(wire + b"\xa3\x06" * depth + b"\xa4\x06" * depth)
