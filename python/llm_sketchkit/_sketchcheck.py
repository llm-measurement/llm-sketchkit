# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Ordered metadata checks using each sketch's existing public error classes."""

from __future__ import annotations

from typing import Any

from . import _proto, profiles


def header(metadata: Any, kind: int, invalid_wire: type[ValueError]) -> None:
    if metadata.kind != kind:
        raise invalid_wire("wrong sketch kind")
    if metadata.wire_version != _proto.WIRE_VERSION:
        raise invalid_wire("wrong wire version")
    if metadata.hash_algo != _proto.HASH_ALGORITHM_HMAC_SHA256_64:
        raise invalid_wire("wrong hash algorithm")


def keying(domain: str, algorithm: str, incompatible_merge: type[ValueError]) -> None:
    if domain not in profiles.REGISTERED_DOMAINS:
        raise incompatible_merge("unregistered hash domain")
    if algorithm != profiles.HMAC_SHA256_64:
        raise incompatible_merge("unsupported hash algorithm")
