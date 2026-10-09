# SPDX-License-Identifier: Apache-2.0
# Code authors: Vijay and Codex

from llm_sketchkit import profiles


def test_profile_constants() -> None:
    for config in profiles.HLLPP_PROFILES.values():
        assert 4 <= config.normal_precision <= 25
        assert config.normal_precision <= config.sparse_precision <= 32
        assert config.promotion_threshold > 0
