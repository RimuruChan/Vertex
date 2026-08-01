package api

import (
	"testing"
)

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter()
	rl.window = 60 // 1 分钟(测试)

	// 同 key 连续 10 次通过
	for i := 0; i < 10; i++ {
		if !rl.Allow("user-1") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	// 第 11 次被拒
	if rl.Allow("user-1") {
		t.Error("11th attempt should be rejected")
	}
	// 其他 key 不受影响
	if !rl.Allow("user-2") {
		t.Error("different user should still be allowed")
	}
}
