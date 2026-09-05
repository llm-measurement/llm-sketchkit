# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay Erramilli and Codex
import json
from dataclasses import replace
from pathlib import Path

import pytest
from llm_sketchkit import _proto, bloom, frequentitems, hllpp, minhash, summary

VECTORS = Path(__file__).resolve().parents[1] / "vectors" / "summaries"


def fixture(producer: str, epoch: str, seq: int, count: int) -> summary.Envelope:
    h = hllpp.Sketch("micro")
    f = frequentitems.Sketch("micro")
    b = bloom.Sketch("micro")
    m = minhash.Sketch("micro")
    for i in range(1, count + 1):
        value = (i * 0x9E3779B97F4A7C15) & ((1 << 64) - 1)
        h.add_hash(value)
        f.add_hash(value, 1)
        b.add_hash(value)
        m.add_hash(value)
    sketches = {
        kind: summary.Payload(s.marshal_binary(), kind)
        for kind, s in (
            ("hllpp", h), ("frequent_items", f), ("bloom", b), ("minhash", m)
        )
    }
    return summary.Envelope(
        accounting_id="test-v1", counters={"requests": count},
        emitted_at_unix_nano=120, epoch=epoch, key_id="test-key-v1",
        observed_end_unix_nano=120, observed_start_unix_nano=60,
        producer_id=producer, scope_id="example", sequence=seq, sketches=sketches,
        version=1, window_duration_unix_nano=60, window_start_unix_nano=60,
    )


def test_shared_vectors() -> None:
    vectors = json.loads((VECTORS / "v1.json").read_text())
    for case in vectors["cases"]:
        docs = [fixture(*row) for row in case["documents"]]
        if case.get("error"):
            with pytest.raises(summary.SummaryError):
                summary.combine(docs, ["a", "b"])
            continue
        result = summary.combine(docs, ["a", "b"])
        assert result.counters["requests"] == case["requests"]
        assert result.missing == case["missing"]
        assert frequentitems.parse(
            result.sketches["frequent_items"].data
        ).total_weight() == case["requests"]
        assert bloom.parse(result.sketches["bloom"].data).inserted_count() == (
            case["requests"]
        )
        assert minhash.parse(result.sketches["minhash"].data).populated_count() == (
            case["requests"]
        )
        if case["name"] == "two_producers":
            combined = replace(
                fixture("combined", "offline", 1, 0),
                counters=result.counters, sketches=result.sketches,
            )
            assert combined.marshal_binary() == (VECTORS / "combined.json").read_bytes()


def test_canonical_and_untrusted_inputs() -> None:
    doc = fixture("a", "one", 1, 2)
    data = doc.marshal_binary()
    assert data == (VECTORS / "envelope.json").read_bytes()
    assert summary.Envelope.parse(data) == doc
    for bad in (
        data + b"\n", data.replace(b'"version":1', b'"version":true'),
        data.replace(b'"version":1', b'"version":1,"version":1'),
        data.replace(b'"version":1', b'"version":1,"unknown":0'),
        b"{", b" " * (summary.MAX_BYTES + 1),
    ):
        with pytest.raises(summary.SummaryError):
            summary.Envelope.parse(bad)
    for field in ("scope_id", "key_id", "accounting_id"):
        other = fixture("b", "two", 1, 2)
        setattr(other, field, "other")
        with pytest.raises(summary.SummaryError):
            summary.combine([doc, other], ["a", "b"])
    other = fixture("b", "two", 1, 2)
    other.sketches["hllpp"] = summary.Payload(b"bad", "hllpp")
    with pytest.raises(summary.SummaryError):
        summary.combine([doc, other], ["a", "b"])


def test_restart_coverage_and_atomicity() -> None:
    a = replace(fixture("a", "one", 1, 2), observed_end_unix_nano=90)
    b = replace(fixture("a", "two", 1, 3), observed_start_unix_nano=90)
    before = a.marshal_binary()
    result = summary.combine([b, a, a], ["a", "missing"])
    assert result.counters["requests"] == 5
    assert result.partial == []
    assert result.missing == ["missing"]
    assert summary.combine(
        [a, replace(b, observed_start_unix_nano=91)], ["a"]
    ).partial == ["a"]
    with pytest.raises(summary.SummaryError):
        summary.combine([a, replace(b, observed_start_unix_nano=89)], ["a"])
    assert a.marshal_binary() == before
    other = fixture("b", "two", 1, 2)
    other.counters["requests"] = (1 << 63) - 1
    with pytest.raises(summary.SummaryError):
        summary.combine([a, other], ["a", "b"])
    other = replace(
        fixture("b", "two", 1, 2), window_start_unix_nano=120,
        observed_start_unix_nano=120, observed_end_unix_nano=180,
        emitted_at_unix_nano=180,
    )
    summary.compatible(a, other)
    with pytest.raises(summary.SummaryError):
        summary.combine([a, other], ["a", "b"])


def test_proto_constants_match_generated_enums() -> None:
    # The quality checker cannot follow cross-module uses of these typed aliases.
    for kind in ("HLLPP", "FREQUENT_ITEMS", "BLOOM", "MINHASH"):
        name = "SKETCH_KIND_" + kind
        assert getattr(_proto, name) == getattr(_proto.sketchpb, name)
    assert _proto.HASH_ALGORITHM_HMAC_SHA256_64 == (
        _proto.sketchpb.HASH_ALGORITHM_HMAC_SHA256_64
    )
    for mode in (
        "HLLPP_SPARSE", "HLLPP_DENSE", "FREQUENT_ITEMS_BOUNDED_MAP",
        "BLOOM_BITSET", "MINHASH_SIGNATURE",
    ):
        assert getattr(_proto, "REPRESENTATION_" + mode) == getattr(
            _proto.sketchpb, "REPRESENTATION_MODE_" + mode
        )
