# SPDX-License-Identifier: Apache-2.0
from __future__ import annotations

import copy
import hashlib
import itertools
import json
from dataclasses import asdict
from fractions import Fraction
from pathlib import Path
from typing import Any

import pytest
from llm_sketchkit import model_usage as mu
from llm_sketchkit import summary
from llm_sketchkit._summary_errors import ModelUsageError

ROOT = Path(__file__).resolve().parents[1] / "vectors/model_usage"


def load(path: str) -> Any:
    return json.loads((ROOT / path).read_bytes())


def cases(name: str) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = load("cases/" + name + ".json")["cases"]
    return result


def reason(exc: Exception) -> str:
    if isinstance(exc, ModelUsageError):
        return exc.reason
    text = str(exc)
    for word, value in (
        ("batch size", "batch_count"),
        ("contract", "contract"),
        ("unexpected", "producer"),
        ("conflicting", "replay_conflict"),
        ("regressed", "regression"),
        ("overflow", "overflow"),
        ("overlapping", "epoch_overlap"),
        ("observation", "interval"),
        ("size", "size"),
    ):
        if word in text:
            return value
    return "structure"


@pytest.mark.parametrize(
    "case", cases("parse") + cases("parse_additions"), ids=lambda c: c["name"]
)
def test_parse_vectors(case: dict[str, Any]) -> None:
    data = (ROOT / case["file"]).read_bytes()
    try:
        parsed = summary.Envelope.parse(data)
    except summary.SummaryError as exc:
        assert not case["accept"], str(exc)
        assert reason(exc) == case["reason"]
    else:
        assert case["accept"]
        assert parsed.marshal_binary() == data


@pytest.mark.parametrize("case", cases("merge"), ids=lambda c: c["name"])
def test_merge_vectors(case: dict[str, Any]) -> None:
    data = [(ROOT / path).read_bytes() for path in case["inputs"]]
    permutations = set(itertools.permutations(data))
    for permutation in permutations:
        docs = [summary.Envelope.parse(b) for b in permutation]
        try:
            result = summary.combine(docs, case["expected_producers"])
        except summary.SummaryError as exc:
            assert not case["accept"], str(exc)
            assert reason(exc) == case["reason"]
        else:
            assert case["accept"]
            projection = {
                "counters": result.counters,
                "model_usage": result.model_usage,
                "missing": result.missing,
                "partial": result.partial,
                "selected_sources": [
                    [s.producer_id, s.epoch, s.sequence] for s in result.sources
                ],
            }
            assert projection == load(case["expected_projection"])
            repeated = summary.combine(docs + docs, case["expected_producers"])
            assert asdict(repeated) == asdict(result)
        assert [d.marshal_binary() for d in docs] == list(permutation)


@pytest.mark.parametrize("path", ["contract.json", "contract-unicode.json"])
def test_contract_vectors(path: str) -> None:
    vector = load(path)
    before = copy.deepcopy(vector)
    assert (
        mu.contract_id(
            vector["contract"]["accounting_id"], vector["model_map"], vector["routing"]
        )
        == vector["contract_id"]
    )
    assert (
        mu.canonical_config(vector["contract"]).decode()
        == vector["canonical_contract_utf8"]
    )
    if "canonical_map_hex" in vector:
        assert (
            mu.canonical_config(vector["model_map"]).hex()
            == vector["canonical_map_hex"]
        )
    assert vector == before
    vector["routing"]["slices"][0]["from_resource_attributes"] = []
    assert (
        mu.contract_id(
            vector["contract"]["accounting_id"], vector["model_map"], vector["routing"]
        )
        == vector["contract_id"]
    )


@pytest.mark.parametrize("case", cases("collection"), ids=lambda c: c["name"])
def test_collection_vectors(case: dict[str, Any]) -> None:
    contract = load("contract.json")
    block = mu.empty(contract["contract_id"])
    for event in case["events"]:
        old = copy.deepcopy(block)
        model_id = mu.classify_model(
            event.get("observed_model", "source-a"),
            contract["model_map"],
            event.get("route", ""),
        )
        observations = {
            k: mu.Observation(event[k], conflict=event.get(k + "_conflict", False))
            for k in mu.FIELDS
        }
        origin = event.get("provenance", "unknown")
        new = mu.collect(block, model_id, observations, origin, origin)
        assert old == block
        block = new
    assert block["models"] == {case["expected_model_id"]: case["expected_row"]}
    if "forbidden_output" in case:
        assert case["forbidden_output"] not in json.dumps(block)


def test_public_data_manifest() -> None:
    assert (
        hashlib.sha256((ROOT / "manifest.json").read_bytes()).hexdigest()
        == "ffabccaaa18b6fb38d947d463abbf2d644f85767efe808b1a820fa56722e2562"
    )
    manifest = load("manifest.json")
    assert manifest["payload_files"] == len(manifest["files"]) == 82
    assert (
        manifest["wire_fixtures"] == len(list((ROOT / "fixtures").glob("*.json"))) == 65
    )
    for record in manifest["files"]:
        data = (ROOT / record["path"]).read_bytes()
        assert len(data) == record["size"]
        assert hashlib.sha256(data).hexdigest() == record["sha256"]
    assert sum(len(cases(p.stem)) for p in (ROOT / "cases").glob("*.json")) == 112


@pytest.mark.parametrize("case", cases("limits"), ids=lambda c: c["name"])
def test_limit_vectors(case: dict[str, Any]) -> None:
    name = case["name"]
    if case.get("accept"):
        summary.Envelope.parse((ROOT / case["fixture"]).read_bytes())
        return
    with pytest.raises(summary.SummaryError) as caught:
        if name == "wire_bytes":
            summary.Envelope.parse(b" " * case["construction"]["count"])
        elif name == "input_batch_count":
            doc = summary.Envelope.parse((ROOT / case["template"]).read_bytes())
            summary.combine([doc] * case["repeat"], [doc.producer_id])
        elif name == "merged_union_limit":
            template = load(case["template"])
            row = template["model_usage"]["models"]["model-a"]
            docs = []
            recipe = case["construction"]
            for (start, end), producer in zip(
                recipe["ranges"], recipe["producer_ids"], strict=True
            ):
                value = copy.deepcopy(template)
                value["producer_id"] = producer
                value["model_usage"]["models"] = {
                    recipe["key_format"] % n: row for n in range(start, end + 1)
                }
                value["counters"] = {
                    k: v * (end - start + 1) for k, v in value["counters"].items()
                }
                docs.append(summary.Envelope.parse(mu.canonical_config(value)))
            before = [d.marshal_binary() for d in docs]
            try:
                summary.combine(docs, recipe["producer_ids"])
            finally:
                assert [d.marshal_binary() for d in docs] == before
        else:
            value = load(case["template"] if "template" in case else case["fixture"])
            recipe = case.get("construction", {})
            if name == "real_model_limit_before_rows":
                value["model_usage"]["models"] = {
                    recipe["key_format"] % n: {}
                    for n in range(recipe["start"], recipe["count"])
                }
            elif name == "input_limits_before_allocation":
                value["model_usage"]["models"] = recipe["value"]
            elif name == "too_long_model_id":
                models = value["model_usage"]["models"]
                models[recipe["repeat_ascii"] * recipe["count"]] = models.pop(
                    recipe["from"]
                )
            summary.Envelope.parse(mu.canonical_config(value))
    expected = {"model_id": "structure"}.get(case["reject"], case["reject"])
    assert reason(caught.value) == expected


def price_example(
    doc: summary.Envelope, case: dict[str, Any]
) -> tuple[Fraction, Fraction]:
    """Test-only interpretation, not a production price-file or reader API."""
    assert doc.model_usage is not None
    if any(k in mu.RESERVED for k in doc.model_usage["models"]):
        raise ValueError("unpriced_reserved_bucket")
    f, g, share = (
        Fraction(case[k]) for k in ("input_factor", "output_factor", "fraction")
    )
    lower = upper = Fraction(0)
    for row in doc.model_usage["models"].values():
        for bin_ in row["histogram"]:
            c = bin_["complete"]
            if not c["requests"]:
                continue
            lo, hi = f * c["input_min"], f * c["input_max"]
            intersecting = []
            previous = None
            covered = False
            for tier in case["tiers"]:
                edge = tier["upper"]
                if (previous is None or hi > previous) and (edge is None or lo <= edge):
                    if set(tier["rates"]) != {
                        "uncached_input",
                        "cache_read",
                        "cache_write",
                        "output",
                    }:
                        raise ValueError("unknown_intersecting_tier")
                    intersecting.append(tier["rates"])
                if edge is None or hi <= edge:
                    covered = True
                    break
                previous = edge
            if not covered:
                raise ValueError("unknown_intersecting_tier")
            components = {
                "uncached_input": f
                * (
                    c["input_tokens"]
                    - c["cache_read_input_tokens"]
                    - c["cache_write_input_tokens"]
                ),
                "cache_read": f * c["cache_read_input_tokens"],
                "cache_write": f * c["cache_write_input_tokens"],
                "output": g * c["output_tokens"],
            }
            for k, v in components.items():
                rates = [Fraction(t[k]) for t in intersecting]
                lower += share * v * min(rates)
                upper += share * v * max(rates)
    return lower, upper


@pytest.mark.parametrize("case", cases("pricing"), ids=lambda c: c["name"])
def test_pricing_interpretation_vectors(case: dict[str, Any]) -> None:
    doc = summary.Envelope.parse((ROOT / case["fixture"]).read_bytes())
    if "reject" in case:
        with pytest.raises(ValueError, match=case["reject"]):
            price_example(doc, case)
    else:
        assert price_example(doc, case) == tuple(
            Fraction(case["expected_micro_usd"][k]) for k in ("lower", "upper")
        )


@pytest.mark.parametrize("case", cases("compatibility"), ids=lambda c: c["name"])
def test_compatibility_vectors(case: dict[str, Any]) -> None:
    name = case["name"]
    if name == "existing_readers_reject":
        # The baseline 14-member bound is also verified with released readers
        # by the local evidence runner. No new diagnostic is attributed to them.
        assert len(load(case["fixture"])) == 15
        return
    if name == "key_team_scope_unavailable":
        doc = summary.Envelope.parse((ROOT / case["fixture"]).read_bytes())
        assert doc.model_usage is not None
        assert set(doc.model_usage) == {
            "version",
            "histogram_id",
            "contract_id",
            "models",
        }
        return
    if "bytes" in case:
        with pytest.raises(summary.SummaryError) as caught:
            summary.has_model_usage(case["bytes"].encode())
        assert all(s not in str(caught.value) for s in case["must_not_emit"])
        return
    data = (ROOT / case["fixture"]).read_bytes() if "fixture" in case else b""
    if name == "disabled_unchanged":
        assert not summary.has_model_usage(data)
        assert summary.Envelope.parse(data).marshal_binary() == data
    elif name == "capability_preflight":
        assert summary.has_model_usage(data)
        assert case["diagnostic"] == "summary includes model_usage; upgrade fleetdiff"
    else:
        with pytest.raises(summary.SummaryError) as caught:
            if "inputs" in case:
                docs = [
                    summary.Envelope.parse((ROOT / p).read_bytes())
                    for p in case["inputs"]
                ]
                summary.combine(docs, ["p1", "p2"])
            else:
                summary.Envelope.parse(data)
        expected = {
            "unsupported_extension": (
                "summary includes unsupported model_usage; upgrade fleetdiff"
            ),
            "presence": (
                "summaries mix model_usage-enabled and legacy files; select one "
                "compatible period or re-export with matching settings"
            ),
            "contract": (
                "summaries have incompatible model_usage contracts; "
                "use matching producer settings"
            ),
        }
        assert expected[reason(caught.value)] == case["diagnostic"]


def test_atomic_overflow_clone_and_privacy() -> None:
    doc = summary.Envelope.parse((ROOT / "fixtures/max_integer.json").read_bytes())
    before = doc.marshal_binary()
    assert doc.model_usage is not None
    with pytest.raises(ModelUsageError, match="overflow"):
        mu.collect(
            doc.model_usage, "model-a", {k: mu.Observation(1) for k in mu.FIELDS}
        )
    assert doc.marshal_binary() == before
    other = copy.deepcopy(doc.model_usage)
    other["models"] = {"model-b": other["models"]["model-a"]}
    with pytest.raises(ModelUsageError, match="overflow"):
        mu.merge(doc.model_usage, other)
    malformed = copy.deepcopy(doc.model_usage)
    malformed["models"]["model-a"]["requests"] = 0
    with pytest.raises(ModelUsageError):
        mu.merge(None, malformed)
    clone = mu.merge(None, doc.model_usage)
    clone["models"]["model-a"]["complete"]["input_min"] = 0
    assert doc.marshal_binary() == before
    contract = load("contract.json")
    for raw in ("\ud800", "\udfff"):
        with pytest.raises(ModelUsageError) as caught:
            mu.contract_id("acct-v1", {raw: "model-a"}, contract["routing"])
        assert raw not in str(caught.value)
    for target in ("__overflow__", "MODEL_PRIVACY_SENTINEL@example.invalid"):
        with pytest.raises(ModelUsageError) as caught:
            mu.contract_id("acct-v1", {"source": target}, contract["routing"])
        assert "PRIVACY_SENTINEL" not in str(caught.value)


@pytest.mark.parametrize(
    "old,new",
    [
        (b"key-v1", bytes([255])),
        (b'"sequence":1', b'"sequence":NaN'),
        (b'"sequence":1', b'"sequence":Infinity'),
        (b'"sequence":1', b'"sequence":-Infinity'),
        (b"key-v1", rb"\ud800"),
        (b"key-v1", rb"\udfff"),
        (b"key-v1", rb"\ud800\u0041"),
    ],
)
def test_preflight_malformed_encoding(old: bytes, new: bytes) -> None:
    valid = (ROOT / "fixtures/one.json").read_bytes()
    data = valid.replace(old, new, 1)
    with pytest.raises(summary.SummaryError):
        summary.has_model_usage(data)
    with pytest.raises(summary.SummaryError):
        summary.Envelope.parse(data)
    assert summary.has_model_usage(valid)


@pytest.mark.parametrize(
    "replacement", [rb"\ud83d\ude00", rb"\\ud800", rb"\ufffd", "\U0001f600".encode()]
)
def test_preflight_valid_unicode(replacement: bytes) -> None:
    valid = (ROOT / "fixtures/one.json").read_bytes()
    assert summary.has_model_usage(valid.replace(b"key-v1", replacement, 1))
