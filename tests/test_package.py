# Code authors: Vijay Erramilli and Codex
from __future__ import annotations

import hashlib
import json
import math
import os
from pathlib import Path
from typing import Any, TypeVar, cast

import llm_sketchkit
from llm_sketchkit import (
    _proto,
    bloom,
    canon,
    frequentitems,
    hash,
    hashfamily,
    hllpp,
    minhash,
    profiles,
)

ROOT = Path(__file__).resolve().parents[1]
T = TypeVar("T")


def test_package_exports_first_class_modules() -> None:
    assert "hllpp" in llm_sketchkit.__all__
    assert "frequentitems" in llm_sketchkit.__all__
    assert "bloom" in llm_sketchkit.__all__
    assert "minhash" in llm_sketchkit.__all__


def test_hash_vectors() -> None:
    for path in sorted((ROOT / "vectors" / "hash").glob("*.json")):
        vector = load_json(path)
        input_value = as_dict(vector["input"])
        secret_value = as_dict(vector["secret"])
        expected = as_dict(vector["expected"])

        canonical = canon.canonicalize(
            as_str(vector["canonicalization"]),
            as_str(input_value["value"]),
        )
        assert canonical.hex() == as_str(vector["canonical_bytes_hex"])

        secret = hash.Secret(as_str(secret_value["utf8"]).encode("utf-8"))
        assert (
            hash.digest64_hex(secret, as_str(vector["domain"]), canonical)
            == expected["digest64_hex"]
        )
        assert (
            hash.hash64(secret, as_str(vector["domain"]), canonical)
            == expected["digest64_uint"]
        )


def test_reserved_canonicalization_profiles_rejected() -> None:
    for profile in [
        "text_v1_casefold",
        "text_v1_fold_ws",
        "text_v1_casefold_fold_ws",
    ]:
        try:
            canon.canonicalize(profile, "hello")
        except canon.UnsupportedProfileError:
            pass
        else:
            raise AssertionError(f"reserved profile {profile!r} was accepted")


def test_secret_from_env_redacts_and_rejects_missing() -> None:
    os.environ["LLM_SKETCHKIT_PY_SECRET"] = "super-secret-value"
    secret = hash.secret_from_env("LLM_SKETCHKIT_PY_SECRET")
    assert "super-secret-value" not in str(secret)
    assert "super-secret-value" not in repr(secret)
    assert not hasattr(secret, "value")

    os.environ.pop("LLM_SKETCHKIT_PY_MISSING", None)
    try:
        hash.secret_from_env("LLM_SKETCHKIT_PY_MISSING")
    except hash.EmptySecretError:
        pass
    else:
        raise AssertionError("missing secret env was accepted")

    for value in ["short", "demo-secret-change-me", "dev-secret-change-me"]:
        os.environ["LLM_SKETCHKIT_PY_WEAK_SECRET"] = value
        try:
            hash.secret_from_env("LLM_SKETCHKIT_PY_WEAK_SECRET")
        except hash.WeakSecretError:
            pass
        else:
            raise AssertionError(f"weak secret {value!r} was accepted")


def test_hashfamily_seeds_and_python_u64_masking() -> None:
    assert hashfamily.BLOOM_SEED == seed_from_tag(b"llm-sketchkit:bloom:v1")
    assert hashfamily.MINHASH_SEED == seed_from_tag(b"llm-sketchkit:minhash:v1")
    assert hashfamily.BLOOM_SEED == 0x83984A98FD448A39
    assert hashfamily.MINHASH_SEED == 0xEA59F2718F8069A6
    assert hashfamily.mix64(-1) == hashfamily.mix64(hashfamily.MASK64)
    assert hashfamily.mix64(1 << 64) == hashfamily.mix64(0)
    assert hashfamily.bloom_hash(1 << 65, 0) == hashfamily.bloom_hash(0, 0)


def test_hllpp_sketch_vectors() -> None:
    for path in sorted((ROOT / "vectors" / "sketches").glob("hllpp_*.json")):
        vector = load_json(path)
        sketch = build_hllpp(vector)
        body = expected_body(vector)

        assert sketch.representation_mode() == as_str(body["representation_mode"])
        if "sparse_count" in body:
            assert sketch.sparse_count() == as_int(body["sparse_count"])
        if "dense_nonzero_count" in body:
            assert sketch.dense_nonzero_count() == as_int(body["dense_nonzero_count"])
        if "sparse_registers" in body:
            assert sketch.sparse_registers() == [
                (as_int(register["index"]), as_int(register["value"]))
                for register in as_list(body["sparse_registers"])
            ]
        assert_stable_hllpp_reserialization(sketch)
        assert_serialized_hex(vector, sketch.marshal_binary(), hllpp.parse)


def test_hllpp_rejects_malformed_wire_state() -> None:
    malformed = {
        "precision_wrap": hllpp_message(p=270, sp=274, sparse=[(1, 1)]),
        "sparse_index_oob": hllpp_message(sparse=[(1 << 18, 1)]),
        "sparse_count_oob": hllpp_message(
            profile="micro",
            p=12,
            sp=16,
            sparse=[(i, 1) for i in range(257)],
        ),
        "dense_rank_invalid": hllpp_message(
            mode=_proto.representation_hllpp_dense(),
            dense=bytes([64 - 14 + 2]) + bytes((1 << 14) - 1),
        ),
        "too_large": bytes(_proto.MAX_WIRE_BYTES + 1),
    }

    for name, data in malformed.items():
        try:
            hllpp.parse(data)
        except hllpp.HLLPPError:
            pass
        else:
            raise AssertionError(f"malformed HLL++ wire case {name!r} was accepted")


def test_frequent_items_sketch_vectors() -> None:
    for path in sorted(
        (ROOT / "vectors" / "sketches").glob("frequent_items_*.json")
    ):
        vector = load_json(path)
        sketch = build_frequent_items(vector)
        body = expected_body(vector)

        assert sketch.total_weight() == as_int(body["total_weight"])
        assert sketch.max_error() == as_int(body["max_error"])
        want_entries = as_list(body["entries"])
        got_entries = sketch.items()
        assert len(got_entries) == len(want_entries)
        for got, want_any in zip(got_entries, want_entries, strict=True):
            want = as_dict(want_any)
            assert got.hash == int(as_str(want["hash_hex"]), 16)
            assert got.estimate == as_int(want["estimate"])
            assert got.error == as_int(want["error"])
            assert got.lower_bound == as_int(want["lower_bound"])
            assert got.upper_bound == as_int(want["upper_bound"])

        if "no_false_negatives" in body:
            assert item_hexes(
                sketch.frequent_items(frequentitems.NO_FALSE_NEGATIVES)
            ) == as_str_list(body["no_false_negatives"])
        if "no_false_positives" in body:
            assert item_hexes(
                sketch.frequent_items(frequentitems.NO_FALSE_POSITIVES)
            ) == as_str_list(body["no_false_positives"])
        assert_stable_frequent_items_reserialization(sketch)
        assert_serialized_hex(vector, sketch.marshal_binary(), frequentitems.parse)


def test_bloom_sketch_vectors() -> None:
    for path in sorted((ROOT / "vectors" / "sketches").glob("bloom_*.json")):
        vector = load_json(path)
        sketch = build_bloom(vector)
        body = expected_body(vector)

        assert sketch.inserted_count() == as_int(body["inserted_count"])
        if "set_bits" in body:
            assert sketch.set_bits() == [
                as_int(value) for value in as_list(body["set_bits"])
            ]
        if "bitset_hex" in body:
            assert sketch.bitset().hex() == as_str(body["bitset_hex"])
        assert math.isclose(
            sketch.false_positive_estimate(),
            as_float(body["false_positive_estimate"]),
            abs_tol=1e-15,
        )
        for hash_hex in as_str_list(body["may_contain"]):
            assert sketch.may_contain_hash(int(hash_hex, 16))
        for hash_hex in as_str_list(body["may_not_contain"]):
            assert not sketch.may_contain_hash(int(hash_hex, 16))
        assert_stable_bloom_reserialization(sketch)
        assert_serialized_hex(vector, sketch.marshal_binary(), bloom.parse)


def test_minhash_sketch_vectors() -> None:
    for path in sorted((ROOT / "vectors" / "sketches").glob("minhash_*.json")):
        vector = load_json(path)
        sketch, by_source = build_minhash(vector)
        body = expected_body(vector)

        assert sketch.populated_count() == as_int(body["populated_count"])
        assert sketch.signature_hex() == as_str_list(body["signature_hex"])
        if as_float(body.get("jaccard", 0.0)) != 0.0:
            left = by_source["left"]
            right = by_source["right"]
            assert math.isclose(
                left.jaccard_estimate(right),
                as_float(body["jaccard"]),
                abs_tol=1e-15,
            )
        assert_stable_minhash_reserialization(sketch)
        assert_serialized_hex(vector, sketch.marshal_binary(), minhash.parse)


def test_cross_language_merge_vectors() -> None:
    for path in sorted(
        (ROOT / "vectors" / "sketches").glob("cross_language_*.json")
    ):
        vector = load_json(path)
        metadata = as_dict(vector["metadata"])
        kind = as_str(metadata["kind"])
        if kind == "HLLPP":
            assert_hllpp_cross_language_merge(vector)
        elif kind == "BLOOM":
            assert_bloom_cross_language_merge(vector)
        elif kind == "MINHASH":
            assert_minhash_cross_language_merge(vector)
        elif kind == "FREQUENT_ITEMS":
            assert_frequent_items_cross_language_merge(vector)
        else:
            raise AssertionError(f"unsupported cross-language kind {kind!r}")


def test_datasketches_oracle_fixture_shape() -> None:
    fixture = load_json(
        ROOT / "vectors" / "oracles" / "datasketches_frequent_items.json"
    )
    assert fixture["schema_version"] == 1
    assert fixture["oracle"] == "apache-datasketches-python"

    names: set[str] = set()
    for workload_any in as_list(fixture["workloads"]):
        workload = as_dict(workload_any)
        name = as_str(workload["name"])
        assert name not in names
        names.add(name)

        assert as_str(workload["profile"]) in profiles.FREQUENT_ITEMS_MAP_SIZES
        assert as_int(workload["datasketches_lg_max_k"]) > 0
        assert as_int(workload["top_k"]) > 0
        assert as_int(workload["partitions"]) > 1

        kind = as_str(workload["kind"])
        if kind == "zipf":
            assert as_int(workload["updates"]) >= as_int(workload["top_k"])
            assert as_int(workload["key_count"]) > as_int(workload["top_k"])
            assert as_float(workload["alpha"]) > 1.0
            assert isinstance(workload["seed"], int)
        elif kind == "tail_churn":
            assert as_int(workload["updates"]) >= as_int(workload["top_k"])
            assert as_int(workload["heavy_keys"]) >= as_int(workload["top_k"])
            assert as_int(workload["tail_keys"]) > as_int(workload["top_k"])
        else:
            raise AssertionError(f"unsupported oracle workload kind {kind!r}")


def assert_hllpp_cross_language_merge(vector: dict[str, Any]) -> None:
    body = expected_body(vector)
    sources = source_serialized_hex(body)
    left = hllpp.parse(bytes.fromhex(sources["left"]))
    right = hllpp.parse(bytes.fromhex(sources["right"]))
    left.merge(right)

    merged_hex = as_str(body["merged_serialized_hex"])
    assert left.marshal_binary().hex() == merged_hex
    assert left.marshal_binary().hex() == as_str(
        as_dict(vector["expected"])["serialized_hex"]
    )
    assert hllpp.parse(bytes.fromhex(merged_hex)).marshal_binary().hex() == merged_hex
    assert left.representation_mode() == as_str(body["representation_mode"])
    assert left.dense_nonzero_count() == as_int(body["dense_nonzero_count"])


def assert_bloom_cross_language_merge(vector: dict[str, Any]) -> None:
    body = expected_body(vector)
    sources = source_serialized_hex(body)
    left = bloom.parse(bytes.fromhex(sources["left"]))
    right = bloom.parse(bytes.fromhex(sources["right"]))
    left.merge(right)

    merged_hex = as_str(body["merged_serialized_hex"])
    assert left.marshal_binary().hex() == merged_hex
    assert bloom.parse(bytes.fromhex(merged_hex)).marshal_binary().hex() == merged_hex
    assert left.inserted_count() == as_int(body["inserted_count"])
    assert left.set_bit_count() == as_int(body["set_bit_count"])
    assert math.isclose(
        left.false_positive_estimate(),
        as_float(body["false_positive_estimate"]),
        abs_tol=1e-15,
    )


def assert_minhash_cross_language_merge(vector: dict[str, Any]) -> None:
    body = expected_body(vector)
    sources = source_serialized_hex(body)
    left = minhash.parse(bytes.fromhex(sources["left"]))
    right = minhash.parse(bytes.fromhex(sources["right"]))
    left.merge(right)

    merged_hex = as_str(body["merged_serialized_hex"])
    assert left.marshal_binary().hex() == merged_hex
    assert minhash.parse(bytes.fromhex(merged_hex)).marshal_binary().hex() == merged_hex
    assert left.populated_count() == as_int(body["populated_count"])


def assert_frequent_items_cross_language_merge(vector: dict[str, Any]) -> None:
    body = expected_body(vector)
    sources = source_serialized_hex(body)
    left = frequentitems.parse(bytes.fromhex(sources["left"]))
    right = frequentitems.parse(bytes.fromhex(sources["right"]))
    left.merge(right)
    expected = frequentitems.parse(bytes.fromhex(as_str(body["merged_serialized_hex"])))

    assert (
        left.total_weight() == expected.total_weight() == as_int(body["total_weight"])
    )
    assert left.max_error() == expected.max_error() == as_int(body["max_error"])
    for entry_any in as_list(body["entries"]):
        entry = as_dict(entry_any)
        value = int(as_str(entry["hash_hex"]), 16)
        assert (
            left.estimate_hash(value)
            == expected.estimate_hash(value)
            == as_int(entry["estimate"])
        )
        assert (
            left.lower_bound_hash(value)
            == expected.lower_bound_hash(value)
            == as_int(entry["lower_bound"])
        )
        assert (
            left.upper_bound_hash(value)
            == expected.upper_bound_hash(value)
            == as_int(entry["upper_bound"])
        )
    assert item_hexes(
        left.frequent_items(frequentitems.NO_FALSE_NEGATIVES)
    ) == as_str_list(body["no_false_negatives"])
    assert item_hexes(
        left.frequent_items(frequentitems.NO_FALSE_POSITIVES)
    ) == as_str_list(body["no_false_positives"])
    assert expected.marshal_binary().hex() == as_str(body["merged_serialized_hex"])


def build_hllpp(vector: dict[str, Any]) -> hllpp.Sketch:
    metadata = as_dict(vector["metadata"])
    by_source: dict[str, hllpp.Sketch] = {}
    for operation in as_list(vector["operations"]):
        op = as_dict(operation)
        assert op["op"] == "add_hash"
        source = as_str(op.get("source", "default"))
        by_source.setdefault(
            source,
            hllpp.new(as_str(metadata["profile"]), as_str(metadata["hash_domain"])),
        ).add_hash(int(as_str(op["hash_hex"]), 16))
    if not by_source:
        return hllpp.new(as_str(metadata["profile"]), as_str(metadata["hash_domain"]))
    return merge_sources(by_source)


def build_frequent_items(vector: dict[str, Any]) -> frequentitems.Sketch:
    metadata = as_dict(vector["metadata"])
    by_source: dict[str, frequentitems.Sketch] = {}
    for operation in as_list(vector["operations"]):
        op = as_dict(operation)
        assert op["op"] == "add_hash_weight"
        source = as_str(op.get("source", "default"))
        by_source.setdefault(
            source,
            frequentitems.new(
                as_str(metadata["profile"]),
                as_str(metadata["hash_domain"]),
            ),
        ).add_hash(int(as_str(op["hash_hex"]), 16), as_int(op["weight"]))
    if not by_source:
        return frequentitems.new(
            as_str(metadata["profile"]),
            as_str(metadata["hash_domain"]),
        )
    return merge_sources(by_source)


def build_bloom(vector: dict[str, Any]) -> bloom.Sketch:
    metadata = as_dict(vector["metadata"])
    by_source: dict[str, bloom.Sketch] = {}
    for operation in as_list(vector["operations"]):
        op = as_dict(operation)
        assert op["op"] == "add_hash"
        source = as_str(op.get("source", "default"))
        by_source.setdefault(
            source,
            bloom.new(as_str(metadata["profile"]), as_str(metadata["hash_domain"])),
        ).add_hash(int(as_str(op["hash_hex"]), 16))
    if not by_source:
        return bloom.new(as_str(metadata["profile"]), as_str(metadata["hash_domain"]))
    return merge_sources(by_source)


def build_minhash(
    vector: dict[str, Any],
) -> tuple[minhash.Sketch, dict[str, minhash.Sketch]]:
    metadata = as_dict(vector["metadata"])
    by_source: dict[str, minhash.Sketch] = {}
    for operation in as_list(vector["operations"]):
        op = as_dict(operation)
        assert op["op"] == "add_hash"
        source = as_str(op.get("source", "default"))
        by_source.setdefault(
            source,
            minhash.new(as_str(metadata["profile"]), as_str(metadata["hash_domain"])),
        ).add_hash(int(as_str(op["hash_hex"]), 16))
    if not by_source:
        sketch = minhash.new(
            as_str(metadata["profile"]),
            as_str(metadata["hash_domain"]),
        )
        return sketch, by_source
    return merge_sources(by_source), by_source


def merge_sources(by_source: dict[str, T]) -> T:
    names = sorted(by_source)
    merged = cast(Any, by_source[names[0]]).clone()
    for name in names[1:]:
        merged.merge(by_source[name])
    return cast(T, merged)


def assert_stable_hllpp_reserialization(sketch: hllpp.Sketch) -> None:
    first = sketch.marshal_binary()
    assert hllpp.parse(first).marshal_binary() == first


def assert_stable_frequent_items_reserialization(
    sketch: frequentitems.Sketch,
) -> None:
    first = sketch.marshal_binary()
    assert frequentitems.parse(first).marshal_binary() == first


def assert_stable_bloom_reserialization(sketch: bloom.Sketch) -> None:
    first = sketch.marshal_binary()
    assert bloom.parse(first).marshal_binary() == first


def assert_stable_minhash_reserialization(sketch: minhash.Sketch) -> None:
    first = sketch.marshal_binary()
    assert minhash.parse(first).marshal_binary() == first


def item_hexes(items: list[frequentitems.Item]) -> list[str]:
    return [f"{item.hash:016x}" for item in items]


def hllpp_message(
    *,
    profile: str = "small",
    p: int = 14,
    sp: int = 18,
    mode: int | None = None,
    sparse: list[tuple[int, int]] | None = None,
    dense: bytes = b"",
) -> bytes:
    metadata = _proto.sketchpb.SketchMetadata(
        kind=_proto.sketch_kind_hllpp(),
        wire_version=_proto.WIRE_VERSION,
        profile=profile,
        hash_domain="prompt:v1",
        hash_algo=_proto.hash_algorithm_hmac_sha256_64(),
        representation_mode=mode or _proto.representation_hllpp_sparse(),
    )
    metadata.hllpp_normal_precision = p
    metadata.hllpp_sparse_precision = sp
    body = _proto.sketchpb.HllppSketch()
    for index, value in sparse or []:
        body.sparse_registers.append(
            _proto.sketchpb.HllppSparseRegister(index=index, value=value)
        )
    body.dense_registers = dense
    return _proto.serialize(_proto.sketchpb.Sketch(metadata=metadata, hllpp=body))


def source_serialized_hex(body: dict[str, Any]) -> dict[str, str]:
    return cast(dict[str, str], body["source_serialized_hex"])


def assert_serialized_hex(
    vector: dict[str, Any],
    encoded: bytes,
    parse_func: Any,
) -> None:
    expected = as_dict(vector["expected"])
    if "serialized_hex" not in expected:
        return
    serialized_hex = as_str(expected["serialized_hex"])
    assert encoded.hex() == serialized_hex
    parsed = parse_func(bytes.fromhex(serialized_hex))
    assert parsed.marshal_binary().hex() == serialized_hex


def expected_body(vector: dict[str, Any]) -> dict[str, Any]:
    return as_dict(as_dict(vector["expected"])["body"])


def seed_from_tag(tag: bytes) -> int:
    return int.from_bytes(hashlib.sha256(tag).digest()[:8], "big")


def load_json(path: Path) -> dict[str, Any]:
    return cast(dict[str, Any], json.loads(path.read_text(encoding="utf-8")))


def as_dict(value: Any) -> dict[str, Any]:
    return cast(dict[str, Any], value)


def as_list(value: Any) -> list[Any]:
    return cast(list[Any], value)


def as_str_list(value: Any) -> list[str]:
    return cast(list[str], value)


def as_str(value: Any) -> str:
    return cast(str, value)


def as_int(value: Any) -> int:
    return cast(int, value)


def as_float(value: Any) -> float:
    return cast(float, value)
