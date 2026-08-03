# Code authors: Vijay Erramilli and Codex
"""Deterministic sketch-local hash families from `spec/hash.md`."""

from __future__ import annotations

import hashlib

MASK64 = (1 << 64) - 1
BLOOM_TAG = b"llm-sketchkit:bloom:v1"
MINHASH_TAG = b"llm-sketchkit:minhash:v1"

BLOOM_SEED = int.from_bytes(hashlib.sha256(BLOOM_TAG).digest()[:8], "big")
MINHASH_SEED = int.from_bytes(hashlib.sha256(MINHASH_TAG).digest()[:8], "big")


def mix64(value: int) -> int:
    """Return the SplitMix finalizer with unsigned 64-bit overflow."""

    x = value & MASK64
    x = ((x ^ (x >> 30)) * 0xBF58476D1CE4E5B9) & MASK64
    x = ((x ^ (x >> 27)) * 0x94D049BB133111EB) & MASK64
    return (x ^ (x >> 31)) & MASK64


def bloom_hash(value: int, index: int) -> int:
    """Return the Bloom hash-family value for a keyed sketch hash."""

    seed = mix64(BLOOM_SEED ^ (index & MASK64))
    return mix64(value ^ seed)


def minhash(value: int, index: int) -> int:
    """Return the MinHash hash-family value for a keyed sketch hash."""

    seed = mix64(MINHASH_SEED ^ (index & MASK64))
    return mix64(value ^ seed)
