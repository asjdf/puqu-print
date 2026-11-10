package puqu

import (
	"testing"
	"time"

	"tinygo.org/x/bluetooth"
)

// TestConnTimeoutNoOverflow 锁住 ConnectionTimeout(uint16, 0.625ms 步进) 的溢出问题：
// 传 45s 曾回绕成约 4s，导致 Windows 上连接必然超时。
func TestConnTimeoutNoOverflow(t *testing.T) {
	tests := []struct {
		want     time.Duration
		wantUnit bluetooth.Duration
	}{
		{5 * time.Second, 8000},
		{20 * time.Second, 32000},
		{40959 * time.Millisecond, 65534},
		{45 * time.Second, 65535}, // 超上限：钳住而不是回绕
		{10 * time.Minute, 65535}, // 远大于上限
		{0, 1},                    // 兜底给最小正值
	}
	for _, tt := range tests {
		got, effective := connTimeout(tt.want)
		if got != tt.wantUnit {
			t.Errorf("connTimeout(%s) 单位 = %d, want %d", tt.want, got, tt.wantUnit)
		}
		if want := time.Duration(tt.wantUnit) * 625 * time.Microsecond; effective != want {
			t.Errorf("connTimeout(%s) 生效时长 = %s, want %s", tt.want, effective, want)
		}
		if effective > 41*time.Second {
			t.Errorf("connTimeout(%s) 生效时长 %s 超出协议上限", tt.want, effective)
		}
	}
}
