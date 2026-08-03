# Code authors: Vijay Erramilli and Codex
"""Profile constants shared by the pure-Python sketch implementations."""

from __future__ import annotations

from dataclasses import dataclass

HMAC_SHA256_64 = "hmac_sha256_64"
PROMPT_V1 = "prompt:v1"
USER_V1 = "user:v1"
TOOL_V1 = "tool:v1"
RETRIEVAL_DOC_V1 = "retrieval-doc:v1"
SESSION_V1 = "session:v1"
MCP_SESSION_V1 = "mcp-session:v1"
MCP_METHOD_V1 = "mcp-method:v1"
TOOL_ERROR_V1 = "tool-error:v1"

REGISTERED_DOMAINS = frozenset(
    {
        PROMPT_V1,
        USER_V1,
        TOOL_V1,
        RETRIEVAL_DOC_V1,
        SESSION_V1,
        MCP_SESSION_V1,
        MCP_METHOD_V1,
        TOOL_ERROR_V1,
    }
)


@dataclass(frozen=True)
class HLLPPProfile:
    normal_precision: int
    sparse_precision: int
    promotion_threshold: int


@dataclass(frozen=True)
class BloomProfile:
    rated_insertions: int
    target_fpr: float
    bit_count: int
    hash_count: int


HLLPP_PROFILES: dict[str, HLLPPProfile] = {
    "micro": HLLPPProfile(12, 16, 1 << (12 - 4)),
    "small": HLLPPProfile(14, 18, 1 << (14 - 4)),
    "default": HLLPPProfile(15, 20, 1 << (15 - 4)),
}

FREQUENT_ITEMS_MAP_SIZES: dict[str, int] = {
    "micro": 256,
    "small": 512,
    "default": 1024,
}

BLOOM_PROFILES: dict[str, BloomProfile] = {
    "micro": BloomProfile(10_000, 0.001, 143_776, 10),
    "small": BloomProfile(100_000, 0.001, 1_437_759, 10),
    "default": BloomProfile(1_000_000, 0.0001, 19_170_117, 13),
}

MINHASH_SIGNATURE_LENGTHS: dict[str, int] = {
    "micro": 64,
    "small": 128,
    "default": 128,
    "k256": 256,
}
