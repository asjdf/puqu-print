//go:build !windows

package puqu

import (
	"fmt"
	"strings"

	"tinygo.org/x/bluetooth"
)

// ParseAddress 解析平台原生的设备地址：macOS 上是 UUID，Linux/HCI 上是 MAC。
// 解析失败时返回零值地址的错误，避免把笔误当成有效地址去连。
func ParseAddress(s string) (bluetooth.Address, error) {
	s = strings.TrimSpace(s)
	var addr bluetooth.Address
	addr.Set(s)
	if addr == (bluetooth.Address{}) {
		return bluetooth.Address{}, fmt.Errorf("无法解析蓝牙地址 %q（macOS 需要设备 UUID，可在 scan 输出里取）", s)
	}
	return addr, nil
}
