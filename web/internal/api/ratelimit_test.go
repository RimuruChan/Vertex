package api

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter()
	// 缩短窗口为 1 秒:时间单位是纳秒,直接写 60 会变成 60ns,导致测试依赖时钟精度
	rl.window = time.Second

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

// TestRateLimiterWindowExpiry:窗口过期后配额恢复。
func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := newRateLimiter()
	rl.window = time.Millisecond

	for i := 0; i < 10; i++ {
		rl.Allow("user-1")
	}
	if rl.Allow("user-1") {
		t.Error("should be rejected within window")
	}
	// 等待窗口过期
	time.Sleep(2 * time.Millisecond)
	if !rl.Allow("user-1") {
		t.Error("should be allowed after window expiry")
	}
}
