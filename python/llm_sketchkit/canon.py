# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex
"""Canonicalization profiles for llm-sketchkit."""

from __future__ import annotations

import unicodedata
from functools import lru_cache

TEXT_V1 = "text_v1"

_WHITE_SPACE = (
    "\u0009\u000a\u000b\u000c\u000d\u0020\u0085\u00a0\u1680"
    "\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a"
    "\u2028\u2029\u202f\u205f\u3000"
)
# Unicode 15 additions to the Unicode 14 database shipped in Python 3.11.
# They have no canonical decompositions or compositions, only new CCC values.
_CCC_15 = {
    "\U00010efd": 220,
    "\U00010efe": 220,
    "\U00010eff": 220,
    "\U00011f41": 9,
    "\U00011f42": 9,
    "\U0001e08f": 230,
    "\U0001e4ec": 232,
    "\U0001e4ed": 232,
    "\U0001e4ee": 220,
    "\U0001e4ef": 230,
}
_MISSING_CCC = frozenset(c for c in _CCC_15 if not unicodedata.combining(c))
# CCC-zero characters that can compose backwards in Unicode 15, after NFKD.
# Go counts these as non-starters too (including modern Jamo V and T).
_BACKWARD_COMBINING = (
    frozenset(
        "\u09be\u09d7\u0b3e\u0b56\u0b57\u0bbe\u0bd7\u0cc2\u0cd5\u0cd6"
        "\u0d3e\u0d57\u0dcf\u0ddf\u102e\u1b35\U00011127\U0001133e"
        "\U00011357\U000114b0\U000114ba\U000114bd\U000115af\U00011930"
    )
    | frozenset(map(chr, range(0x1161, 0x1176)))
    | frozenset(map(chr, range(0x11A8, 0x11C3)))
)


class CanonicalizationError(ValueError):
    """Base class for canonicalization failures."""


class UnsupportedProfileError(CanonicalizationError):
    """Raised when a canonicalization profile is not implemented."""


class InvalidUTF8Error(CanonicalizationError):
    """Raised when input bytes are not valid UTF-8."""


def canonicalize(profile: str, value: bytes | str) -> bytes:
    """Canonicalize a UTF-8 text value under the named profile."""

    if profile != TEXT_V1:
        raise UnsupportedProfileError("unsupported canonicalization profile")
    text = _decode_text(value)
    return _canonicalize_text_v1(text).encode("utf-8")


def canonicalize_text_v1(value: bytes | str) -> bytes:
    """Canonicalize a UTF-8 text value under `text_v1`."""

    return canonicalize(TEXT_V1, value)


def _decode_text(value: bytes | str) -> str:
    if isinstance(value, str):
        try:
            value.encode("utf-8")
        except UnicodeEncodeError:
            raise InvalidUTF8Error("invalid UTF-8 string") from None
        return value

    try:
        return value.decode("utf-8")
    except UnicodeDecodeError:
        raise InvalidUTF8Error("invalid UTF-8 bytes") from None


def _canonicalize_text_v1(value: str) -> str:
    normalized = _stream_safe_nfc(value)
    normalized = normalized.replace("\r\n", "\n").replace("\r", "\n")
    normalized = normalized.strip(_WHITE_SPACE)
    return _stream_safe_nfc(normalized)


def _combining(char: str) -> int:
    return _CCC_15.get(char, unicodedata.combining(char))


@lru_cache(maxsize=4096)
def _nonstarter_counts(char: str) -> tuple[int, int]:
    # Count compatibility decomposition, but keep the original character in NFC.
    decomposition = unicodedata.normalize("NFKD", char)
    leading = trailing = 0
    for part in decomposition:
        if not (_combining(part) or part in _BACKWARD_COMBINING):
            break
        leading += 1
    for part in reversed(decomposition):
        if not (_combining(part) or part in _BACKWARD_COMBINING):
            break
        trailing += 1
    return leading, trailing


def _stream_safe_nfc(value: str) -> str:
    if value.isascii():
        return value
    parts: list[str] = []
    count = 0
    for char in value:
        leading, trailing = _nonstarter_counts(char)
        if count + leading > 30:
            parts.append("\u034f")
            count = 0
        count = count + leading if leading else trailing
        parts.append(char)
    return _nfc("".join(parts))


def _nfc(value: str) -> str:
    if not _MISSING_CCC or not any(c in _MISSING_CCC for c in value):
        return unicodedata.normalize("NFC", value)

    # Python 3.11 treats the ten new marks as starters. Reorder with their
    # corrected CCC, then compose through stdlib pairs with normal blocking.
    ordered: list[str] = []
    segment = 0
    for char in unicodedata.normalize("NFD", value):
        ccc = _combining(char)
        position = len(ordered)
        if ccc:
            while position > segment and _combining(ordered[position - 1]) > ccc:
                position -= 1
        else:
            segment = position + 1
        ordered.insert(position, char)

    composed: list[str] = []
    starter = -1
    previous_ccc = 0
    for char in ordered:
        ccc = _combining(char)
        if starter >= 0 and (previous_ccc < ccc or previous_ccc == 0):
            pair = unicodedata.normalize("NFC", composed[starter] + char)
            if len(pair) == 1:
                composed[starter] = pair
                continue
        if ccc == 0:
            starter = len(composed)
        previous_ccc = ccc
        composed.append(char)
    return "".join(composed)
