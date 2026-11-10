package puqu

import (
	"encoding/hex"
	"testing"
)

// 真机抓包：AQ-V26xxxxxxx 上 readDeviceDetails / readDeviceState 的原始回包。
const (
	realDetails = "3a00006210100400133003000000005a"
	realState   = "3a0094003c3c000a"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func TestParseRealDetails(t *testing.T) {
	raw := mustHex(t, realDetails)
	d, ok := ParseDetails(raw)
	if !ok {
		t.Fatal("真实详情回包未被识别")
	}
	if d.Darkness != 7 || d.Speed != 3 {
		t.Errorf("浓度/速度 = %d/%d, want 7/3", d.Darkness, d.Speed)
	}
	if d.PaperType != 2 || d.PaperFeed != 1 {
		t.Errorf("纸张/进纸 = %d/%d, want 2/1", d.PaperType, d.PaperFeed)
	}
	if d.Close != 2 || d.Language != 1 {
		t.Errorf("关机/语言 = %d/%d, want 2/1", d.Close, d.Language)
	}
	if d.Sound != 1 || d.BatteryLevel != 4 {
		t.Errorf("声音/电量 = %d/%d, want 1/4", d.Sound, d.BatteryLevel)
	}
	if d.Point != Point200 {
		t.Errorf("点位 = %d, want 200", d.Point)
	}
}

func TestParseRealState(t *testing.T) {
	raw := mustHex(t, realState)
	s, ok := ParseState(raw)
	if !ok {
		t.Fatal("真实状态回包未被识别")
	}
	if s.CacheSize != 0x3C {
		t.Errorf("缓存 = %d, want 60", s.CacheSize)
	}
	if s.Printing || s.PrintError || s.LackOfPaper {
		t.Errorf("状态位异常: %+v", s)
	}
	if len(s.Issues()) != 0 {
		t.Errorf("不应有异常: %v", s.Issues())
	}
}

func TestParseRejectsOtherPackets(t *testing.T) {
	// 详情包会被状态解析器拒绝，反之亦然。
	d := mustHex(t, realDetails)
	if _, ok := ParseState(d); ok {
		t.Error("详情包被误判为状态包")
	}
	s := mustHex(t, realState)
	if _, ok := ParseDetails(s); ok {
		t.Error("状态包被误判为详情包")
	}
	if _, ok := ParseDetails([]byte{1, 2, 3}); ok {
		t.Error("短包不应被识别")
	}
}

func TestParseStateFlags(t *testing.T) {
	// 0x80 缺纸 + 0x08 打印中 + 0x10 低电量。
	raw := []byte{0x3A, 0x98, 0x00, 0x05, 0x02, 0x00, 0x30, 0x0A}
	s, ok := ParseState(raw)
	if !ok {
		t.Fatal("状态包未被识别")
	}
	if !s.LackOfPaper || !s.Printing || !s.LowPower {
		t.Errorf("状态位解析错误: %+v", s)
	}
	if s.ProcessedPages != 5 || s.CacheSize != 2 || s.WarningCode != 3 {
		t.Errorf("计数解析错误: %+v", s)
	}
	if s.Ready() {
		t.Error("缺纸 + 打印中不应为就绪")
	}
	if len(s.Issues()) != 3 {
		t.Errorf("异常项 = %v, want 3 项", s.Issues())
	}
}
