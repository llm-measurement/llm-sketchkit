// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package hllpp

import "testing"

func TestProfileConstants(t *testing.T) {
	for name, config := range profileConfigs {
		if config.p < 4 || config.p > 25 || config.sp < config.p || config.sp > 32 || config.promotionThreshold <= 0 {
			t.Fatalf("invalid trusted profile %s", name)
		}
	}
}
