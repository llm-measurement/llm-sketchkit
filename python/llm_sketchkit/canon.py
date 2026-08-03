# Code authors: Vijay Erramilli and Codex
"""Canonicalization profiles for llm-sketchkit."""

from __future__ import annotations

import unicodedata

TEXT_V1 = "text_v1"


class CanonicalizationError(ValueError):
    """Base class for canonicalization failures."""


class UnsupportedProfileError(CanonicalizationError):
    """Raised when a canonicalization profile is not implemented."""


class InvalidUTF8Error(CanonicalizationError):
    """Raised when input bytes are not valid UTF-8."""


def canonicalize(profile: str, value: bytes | str) -> bytes:
    """Canonicalize a UTF-8 text value under the named profile."""

    if profile != TEXT_V1:
        raise UnsupportedProfileError(profile)
    text = _decode_text(value)
    return _canonicalize_text_v1(text).encode("utf-8")


def canonicalize_text_v1(value: bytes | str) -> bytes:
    """Canonicalize a UTF-8 text value under `text_v1`."""

    return canonicalize(TEXT_V1, value)


def _decode_text(value: bytes | str) -> str:
    if isinstance(value, str):
        try:
            value.encode("utf-8")
        except UnicodeEncodeError as exc:
            raise InvalidUTF8Error("invalid UTF-8 string") from exc
        return value

    try:
        return value.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise InvalidUTF8Error("invalid UTF-8 bytes") from exc


def _canonicalize_text_v1(value: str) -> str:
    normalized = unicodedata.normalize("NFC", value)
    normalized = normalized.replace("\r\n", "\n").replace("\r", "\n")
    normalized = normalized.strip()
    return unicodedata.normalize("NFC", normalized)
