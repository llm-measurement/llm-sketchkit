# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex

import time

from llm_sketchkit import frequentitems, summary


def test_combine_1024_documents() -> None:
    sketch = frequentitems.Sketch("default")
    empty = summary.Payload(sketch.marshal_binary(), "frequent_items")
    for i in range(1, 1025):
        sketch.add_hash(i * 0x9E3779B97F4A7C15, i)
    full = summary.Payload(sketch.marshal_binary(), "frequent_items")
    docs = [summary.Envelope(
        accounting_id="test", counters={"requests": 1}, emitted_at_unix_nano=1024,
        epoch=f"e{i:04d}", key_id="test-key", observed_start_unix_nano=i,
        observed_end_unix_nano=i + 1, producer_id="a", scope_id="test", sequence=1,
        sketches={f"s{j:02d}": full if i == 0 else empty for j in range(16)},
        version=1, window_duration_unix_nano=1024, window_start_unix_nano=0,
    ) for i in range(1024)]
    start = time.monotonic()
    result = summary.combine(docs, ["a"])
    elapsed = time.monotonic() - start
    assert result.counters == {"requests": 1024}
    # Broad enough for shared CI; the former quadratic path takes over a minute.
    assert elapsed < 10, f"combine took {elapsed:.2f}s (10s budget)"
    assert docs[0].sketches["s00"].data == full.data


def test_repeated_counter_updates_keep_heap_bounded() -> None:
    sketch = frequentitems.Sketch("micro")
    for _ in range(10000):
        sketch.add_hash(1, 1)
    assert len(sketch._minimum) <= 2 * sketch.map_size()
    assert sketch.estimate_hash(1) == 10000
    assert sketch.clone().marshal_binary() == sketch.marshal_binary()
