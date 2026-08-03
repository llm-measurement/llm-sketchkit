# Code authors: Vijay Erramilli and Codex
"""Domain-separated keyed hashing for sketch inputs."""

from __future__ import annotations

import hashlib
import hmac
import os
from dataclasses import dataclass

from . import profiles

Algorithm = str
Domain = str

HMAC_SHA256_64 = profiles.HMAC_SHA256_64
MIN_SECRET_BYTES = 16
PROMPT_V1 = profiles.PROMPT_V1
USER_V1 = profiles.USER_V1
TOOL_V1 = profiles.TOOL_V1
RETRIEVAL_DOC_V1 = profiles.RETRIEVAL_DOC_V1
SESSION_V1 = profiles.SESSION_V1
MCP_SESSION_V1 = profiles.MCP_SESSION_V1
MCP_METHOD_V1 = profiles.MCP_METHOD_V1
TOOL_ERROR_V1 = profiles.TOOL_ERROR_V1


class HashError(ValueError):
    """Base class for keyed-hash failures."""


class EmptySecretError(HashError):
    """Raised when no secret material is supplied."""


class EmptySecretEnvError(HashError):
    """Raised when no secret environment variable name is supplied."""


class WeakSecretError(HashError):
    """Raised when secret material is too short or a known placeholder."""


class UnregisteredDomainError(HashError):
    """Raised when a domain is not in the v0.1 registry."""


@dataclass(frozen=True, repr=False)
class Secret:
    """Opaque keyed-hash secret material."""

    _value: bytes

    def __post_init__(self) -> None:
        _validate_secret(self._value)

    def __repr__(self) -> str:
        return "Secret(<redacted>)"

    def __str__(self) -> str:
        return "<redacted hash secret>"


def secret_from_env(env_name: str) -> Secret:
    """Load opaque secret bytes from an environment variable."""

    if not env_name:
        raise EmptySecretEnvError("empty hash secret environment variable")
    value = os.environ.get(env_name)
    if not value:
        raise EmptySecretError(env_name)
    return Secret(value.encode("utf-8"))


def domains() -> list[Domain]:
    """Return registered v0.1 hash domains in spec order."""

    return [
        PROMPT_V1,
        USER_V1,
        TOOL_V1,
        RETRIEVAL_DOC_V1,
        SESSION_V1,
        MCP_SESSION_V1,
        MCP_METHOD_V1,
        TOOL_ERROR_V1,
    ]


def is_registered_domain(domain: Domain) -> bool:
    """Return whether a hash domain is registered."""

    return domain in profiles.REGISTERED_DOMAINS


def digest64(secret: Secret, domain: Domain, canonical_bytes: bytes) -> bytes:
    """Return the first eight HMAC-SHA256 digest bytes."""

    _validate_secret(secret._value)
    if not is_registered_domain(domain):
        raise UnregisteredDomainError(domain)
    message = domain.encode("ascii") + b"\x00" + canonical_bytes
    return hmac.new(secret._value, message, hashlib.sha256).digest()[:8]


def hash64(secret: Secret, domain: Domain, canonical_bytes: bytes) -> int:
    """Return HMAC-SHA256-64 as an unsigned big-endian integer."""

    return int.from_bytes(digest64(secret, domain, canonical_bytes), "big")


def digest64_hex(secret: Secret, domain: Domain, canonical_bytes: bytes) -> str:
    """Return the lowercase fixed-width hexadecimal digest."""

    return digest64(secret, domain, canonical_bytes).hex()


def _validate_secret(value: bytes) -> None:
    if not value:
        raise EmptySecretError("empty hash secret")
    if len(value) < MIN_SECRET_BYTES:
        raise WeakSecretError(f"hash secret must be at least {MIN_SECRET_BYTES} bytes")
    if value in {b"demo-secret-change-me", b"dev-secret-change-me"}:
        raise WeakSecretError("placeholder secret must be replaced")
