# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
from __future__ import annotations

import base64
import json
import os
import traceback
from collections.abc import Callable
from pathlib import Path
from types import ModuleType
from typing import cast

import pytest
from llm_sketchkit import _proto, bloom, frequentitems, hash, hllpp, minhash, summary

SENTINEL = "PRIVATE_WP3_SENTINEL\n\x1b[31m" * 32
MODULES = [hllpp, frequentitems, bloom, minhash]
FIXTURE = Path(__file__).resolve().parents[1] / "vectors/summaries/envelope.json"


def assert_safe_error(
    operation: Callable[[], object], expected: type[Exception]
) -> None:
    try:
        operation()
    except Exception as exc:
        assert isinstance(exc, expected)
        assert len(str(exc)) <= 160
        for text in (str(exc), repr(exc), traceback.format_exc()):
            assert "PRIVATE_WP3_SENTINEL" not in text
            assert "\x1b" not in text
        assert "\n" not in str(exc)
        assert "\r" not in str(exc)
        assert exc.__cause__ is None
        if exc.__context__ is not None:
            assert exc.__suppress_context__
    else:
        pytest.fail("untrusted input was accepted")


@pytest.mark.parametrize("module", MODULES)
@pytest.mark.parametrize("field", ["profile", "domain", "algorithm"])
def test_constructor_metadata_errors(module: ModuleType, field: str) -> None:
    args = {"profile": "micro", "domain": hash.PROMPT_V1,
            "algorithm": hash.HMAC_SHA256_64}
    args[field] = SENTINEL
    expected = (
        module.UnknownProfileError
        if field == "profile" else module.IncompatibleMergeError
    )
    assert_safe_error(lambda: module.Sketch(**args), expected)


@pytest.mark.parametrize("module", MODULES)
@pytest.mark.parametrize("field", ["profile", "hash_domain"])
def test_decoder_metadata_errors(module: ModuleType, field: str) -> None:
    message = _proto.parse_sketch(module.Sketch("micro").marshal_binary())
    setattr(message.metadata, field, SENTINEL)
    data = cast(bytes, message.SerializeToString(deterministic=True))
    expected = (
        module.UnknownProfileError
        if field == "profile" else module.IncompatibleMergeError
    )
    assert_safe_error(lambda: module.parse(data), expected)


@pytest.mark.parametrize("module", MODULES)
@pytest.mark.parametrize("field", ["profile", "hash_domain", "malformed"])
def test_summary_tracebacks(module: ModuleType, field: str) -> None:
    kind = ("frequent_items" if module is frequentitems
            else module.__name__.split(".")[-1])
    message = _proto.parse_sketch(module.Sketch("micro").marshal_binary())
    if field != "malformed":
        setattr(message.metadata, field, SENTINEL)
    data = (b"\xff" if field == "malformed"
            else cast(bytes, message.SerializeToString(deterministic=True)))
    envelope = summary.Envelope.parse(FIXTURE.read_bytes())
    valid = summary.Envelope.parse(FIXTURE.read_bytes())
    envelope.sketches[kind] = summary.Payload(data, kind)
    document = json.loads(FIXTURE.read_bytes())
    document["sketches"][kind]["data"] = base64.b64encode(data).decode("ascii")
    encoded = json.dumps(document, sort_keys=True, separators=(",", ":")).encode()
    for operation in (
        envelope.validate, envelope.marshal_binary,
        lambda: summary.Envelope.parse(encoded),
        lambda: summary.compatible(envelope, valid),
        lambda: summary.compatible(valid, envelope),
        lambda: summary.combine([envelope], [envelope.producer_id]),
    ):
        assert_safe_error(operation, summary.SummaryError)


@pytest.mark.parametrize("field", ["unknown", "base64", "utf8"])
def test_summary_json_tracebacks(field: str) -> None:
    document = json.loads(FIXTURE.read_bytes())
    if field == "unknown":
        document[SENTINEL] = 1
    elif field == "base64":
        document["sketches"]["hllpp"]["data"] = SENTINEL + "\u0100"
    encoded = json.dumps(document).encode()
    if field == "utf8":
        encoded = SENTINEL.encode() + b"\xff"
    assert_safe_error(lambda: summary.Envelope.parse(encoded), summary.SummaryError)


@pytest.mark.skipif(not os.supports_bytes_environ, reason="requires byte environment")
def test_secret_raw_environment_bytes(monkeypatch: pytest.MonkeyPatch) -> None:
    raw = b"0123456789abcdef-\xff\xfe-WP3"
    monkeypatch.setitem(os.environb, b"WP3_SECRET", raw)
    secret = hash.secret_from_env("WP3_SECRET")
    # This independent HMAC result is also asserted by the Go test.
    for _ in range(2):
        assert hash.digest64_hex(secret, hash.PROMPT_V1, b"hello") == "a981aba71dfda38f"
    assert hash.digest64(secret, hash.PROMPT_V1, b"hello") == hash.digest64(
        hash.Secret(raw), hash.PROMPT_V1, b"hello"
    )
    for text in (str(secret), repr(secret)):
        assert "0123456789abcdef" not in text
    monkeypatch.setitem(os.environb, b"WP3_SECRET", b"\xff\xfe")
    assert_safe_error(lambda: hash.secret_from_env("WP3_SECRET"), hash.WeakSecretError)


def test_secret_encoding_error_is_suppressed(monkeypatch: pytest.MonkeyPatch) -> None:
    def reject(value: str) -> bytes:
        raise UnicodeEncodeError("utf-8", value, 0, 1, "invalid encoding")

    monkeypatch.setenv("WP3_SECRET", SENTINEL)
    monkeypatch.setattr(os, "fsencode", reject)
    assert_safe_error(lambda: hash.secret_from_env("WP3_SECRET"), hash.HashError)


def test_hash_error_metadata(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv(SENTINEL, "")
    assert_safe_error(lambda: hash.secret_from_env(SENTINEL), hash.EmptySecretError)
    secret = hash.Secret(b"0123456789abcdef-test")
    assert_safe_error(
        lambda: hash.digest64(secret, SENTINEL, b"hello"), hash.UnregisteredDomainError
    )
