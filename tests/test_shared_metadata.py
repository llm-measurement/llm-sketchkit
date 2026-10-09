# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
from __future__ import annotations

from types import ModuleType

import pytest
from llm_sketchkit import _proto, bloom, frequentitems, hllpp, minhash, profiles

MODULES = [hllpp, frequentitems, bloom, minhash]
SENTINEL = "PRIVATE_KEYING_SENTINEL\n\x1b" * 32


@pytest.mark.parametrize("module", MODULES)
def test_constructor_error_order(module: ModuleType) -> None:
    with pytest.raises(module.UnknownProfileError) as caught:
        module.Sketch(SENTINEL, SENTINEL, SENTINEL)
    assert type(caught.value) is module.UnknownProfileError
    assert SENTINEL not in str(caught.value)

    with pytest.raises(module.IncompatibleMergeError) as caught:
        module.Sketch("micro", SENTINEL, SENTINEL)
    assert type(caught.value) is module.IncompatibleMergeError
    assert str(caught.value) == "unregistered hash domain"

    with pytest.raises(module.IncompatibleMergeError) as caught:
        module.Sketch("micro", profiles.PROMPT_V1, SENTINEL)
    assert type(caught.value) is module.IncompatibleMergeError
    assert str(caught.value) == "unsupported hash algorithm"


@pytest.mark.parametrize("module", MODULES)
def test_wire_header_error_order(module: ModuleType) -> None:
    wire = _proto.parse_sketch(module.Sketch("micro").marshal_binary())
    kind = wire.metadata.kind
    wire.metadata.kind = 99
    wire.metadata.wire_version = 2
    wire.metadata.hash_algo = 99
    wire.metadata.profile = SENTINEL
    wire.metadata.hash_domain = SENTINEL

    for field, valid, expected in (
        ("kind", kind, "wrong sketch kind"),
        ("wire_version", _proto.WIRE_VERSION, "wrong wire version"),
        ("hash_algo", _proto.HASH_ALGORITHM_HMAC_SHA256_64, "wrong hash algorithm"),
    ):
        with pytest.raises(module.InvalidWireEncodingError) as caught:
            module.parse(wire.SerializeToString(deterministic=True))
        assert type(caught.value) is module.InvalidWireEncodingError
        assert str(caught.value) == expected
        setattr(wire.metadata, field, valid)


@pytest.mark.parametrize("module", [bloom, minhash])
def test_wire_domain_check_precedes_body_check(module: ModuleType) -> None:
    wire = _proto.parse_sketch(module.Sketch("micro").marshal_binary())
    wire.metadata.hash_domain = SENTINEL
    if module is bloom:
        wire.bloom.bitset = b""
    else:
        del wire.minhash.signature[:]
    with pytest.raises(module.IncompatibleMergeError) as caught:
        module.parse(wire.SerializeToString(deterministic=True))
    assert type(caught.value) is module.IncompatibleMergeError
    assert str(caught.value) == "unregistered domain"
