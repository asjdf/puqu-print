//go:build windows

package puqu

import (
	"fmt"
	"strings"

	"tinygo.org/x/bluetooth"
)

// ParseAddress 解析 "AA:BB:CC:DD:EE:FF" 形式的蓝牙 MAC 地址。
// Windows 后端的 Address 是 MAC 地址，可以直接按 MAC 直连设备。
func ParseAddress(s string) (bluetooth.Address, error) {
	mac, err := bluetooth.ParseMAC(strings.TrimSpace(s))
	if err != nil {
		return bluetooth.Address{}, fmt.Errorf("解析蓝牙地址 %q 失败: %w", s, err)
	}
	return bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, nil
}
