package puqu

import (
	"bytes"
	"testing"
)

func TestPlaceRowsByteAligned(t *testing.T) {
	got := PlaceRows([][]byte{{0xFF}}, 24, 8)
	want := [][]byte{{0x00, 0xFF, 0x00}}
	if !bytes.Equal(got[0], want[0]) {
		t.Errorf("字节对齐放置 = % X, want % X", got[0], want[0])
	}
}

func TestPlaceRowsUnaligned(t *testing.T) {
	got := PlaceRows([][]byte{{0xFF}}, 16, 4)
	// 内容占第 4..11 位，正好跨两个字节。
	want := []byte{0x0F, 0xF0}
	if !bytes.Equal(got[0], want) {
		t.Errorf("非对齐放置 = % X, want % X", got[0], want)
	}
}

func TestPlaceRowsKeepsContent(t *testing.T) {
	content := [][]byte{{0x81}, {0x00}}
	got := PlaceRows(content, 24, 8)
	if len(got) != 2 {
		t.Fatalf("行数 = %d, want 2", len(got))
	}
	if got[0][1] != 0x81 || got[0][0] != 0 || got[0][2] != 0 {
		t.Errorf("内容位置错误: % X", got[0])
	}
	if !bytes.Equal(got[1], []byte{0x00, 0x00, 0x00}) {
		t.Errorf("空行应保持全零: % X", got[1])
	}
}

func TestPlaceRowsClampsOffset(t *testing.T) {
	got := PlaceRows([][]byte{{0xFF}}, 24, 20)
	// 8 点内容放进 24 点行、偏移 20 越界时贴右。
	if !bytes.Equal(got[0], []byte{0x00, 0x00, 0xFF}) {
		t.Errorf("越界偏移应贴右: % X", got[0])
	}
	if got := PlaceRows([][]byte{{0xFF}}, 24, -5); !bytes.Equal(got[0], []byte{0xFF, 0x00, 0x00}) {
		t.Errorf("负偏移应贴左: % X", got[0])
	}
}

func TestPlaceRowsWiderThanLine(t *testing.T) {
	got := PlaceRows([][]byte{{0xFF, 0x81}}, 8, 8)
	if !bytes.Equal(got[0], []byte{0xFF}) {
		t.Errorf("超宽内容应截断且偏移归零: % X", got[0])
	}
}
