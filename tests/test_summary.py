# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
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


def assert_zero_length_state(payload: summary.Payload, empty: bool) -> None:
    doc = fixture("a", "e", 1, 0)
    doc.sketches = {payload.kind: payload}
    doc.validate()  # The same retained state is valid over a positive interval.
    doc.observed_start_unix_nano = doc.observed_end_unix_nano = 90
    if empty:
        doc.validate()
        assert summary.Envelope.parse(doc.marshal_binary()) == doc
        summary.compatible(doc, doc)
        result = summary.combine([doc], ["a"])
        assert result.sketches[payload.kind].data == payload.data
    else:
        with pytest.raises(summary.SummaryError):
            doc.validate()
        with pytest.raises(summary.SummaryError):
            doc.marshal_binary()
        with pytest.raises(summary.SummaryError):
            summary.Envelope.parse(doc._marshal_validated())
        with pytest.raises(summary.SummaryError):
            summary.compatible(doc, doc)
        with pytest.raises(summary.SummaryError):
            summary.combine([doc], ["a"])


@pytest.mark.parametrize("kind", ["hllpp", "frequent_items", "bloom", "minhash"])
@pytest.mark.parametrize("count", [0, 1])
def test_zero_length_sketch_state(kind: str, count: int) -> None:
    payload = fixture("a", "e", 1, count).sketches[kind]
    assert_zero_length_state(payload, empty=count == 0)


@pytest.mark.parametrize("populated", [False, True])
def test_zero_length_dense_hll(populated: bool) -> None:
    sketch = hllpp.Sketch("micro")
    if populated:
        sketch.add_hash(1)
    sketch.force_dense()
    assert_zero_length_state(
        summary.Payload(sketch.marshal_binary(), "hllpp"), empty=not populated
    )


@pytest.mark.parametrize("case", [
    "fi_weight_without_entries", "bloom_count_without_bits",
    "bloom_bits_without_count", "minhash_count_without_signature",
])
def test_zero_length_hidden_retained_state(case: str) -> None:
    kind = {
        "fi_weight_without_entries": "frequent_items",
        "bloom_count_without_bits": "bloom",
        "bloom_bits_without_count": "bloom",
        "minhash_count_without_signature": "minhash",
    }[case]
    payload = fixture("a", "e", 1, 0).sketches[kind]
    message = _proto.parse_sketch(payload.data)
    if case == "fi_weight_without_entries":
        message.frequent_items.total_weight = 1
        message.frequent_items.max_error = 1
    elif case == "bloom_count_without_bits":
        message.bloom.inserted_count = 1
    elif case == "bloom_bits_without_count":
        bits = bytearray(message.bloom.bitset)
        bits[0] = 1
        message.bloom.bitset = bytes(bits)
    else:
        message.minhash.populated_count = 1
    assert_zero_length_state(
        summary.Payload(_proto.serialize(message), kind), empty=False
    )


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
