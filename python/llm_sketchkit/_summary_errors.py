# SPDX-License-Identifier: Apache-2.0
"""Static summary diagnostics. Never include untrusted values."""


class SummaryError(ValueError):
    """Malformed, incompatible, conflicting, or oversized summary input."""


class ModelUsageError(SummaryError):
    def __init__(self, reason: str) -> None:
        self.reason = reason
        super().__init__("invalid model_usage: " + reason)
