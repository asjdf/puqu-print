package puqu

import (
	"bytes"
	"testing"
)

func TestPackRowsBitOrder(t *testing.T) {
	// 8 点宽、2 行：第一行全黑，第二行全白。
	bits := make([]bool, 8*2)
	for i := 0; i < 8; i++ {
		bits[i] = true
	}
	rows, height := PackRows(bits, 8, 2)
	if height != 2 {
		t.Fatalf("height = %d, want 2", height)
	}
	if !bytes.Equal(rows[0], []byte{0xFF}) {
		t.Errorf("黑行 = % X, want FF", rows[0])
	}
	if !bytes.Equal(rows[1], []byte{0x00}) {
		t.Errorf("白行 = % X, want 00", rows[1])
	}

	// 只点亮最左和最右两个像素，验证 MSB 优先。
	edge := make([]bool, 8)
	edge[0], edge[7] = true, true
	rows, _ = PackRows(edge, 8, 1)
	if rows[0][0] != 0x81 {
		t.Errorf("首尾像素 = % X, want 81 (MSB 优先)", rows[0][0])
	}
}

func TestCompressRows(t *testing.T) {
	rows := [][]byte{{0xFF}, {0x00}}
	got := compressRows(rows)
	// 第一行 3C|计数|左零|右零|数据，第二行整行全零时右零 = 行宽。
	want := []byte{
		0x3C, 0x01, 0x00, 0x00, 0x00, 0xFF,
		0x3C, 0x01, 0x00, 0x00, 0x01,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("compressRows =\n  % X\nwant\n  % X", got, want)
	}
}

func TestCompressRowsRunLength(t *testing.T) {
	rows := [][]byte{{0xF0}, {0xF0}, {0xF0}}
	got := compressRows(rows)
	want := []byte{0x3C, 0x03, 0x00, 0x00, 0x00, 0xF0}
	if !bytes.Equal(got, want) {
		t.Errorf("游程合并 =\n  % X\nwant\n  % X", got, want)
	}
}

func TestTrimZeros(t *testing.T) {
	cases := []struct {
		row               []byte
		left, right, tail int
	}{
		{[]byte{0x00, 0x00}, 0, 2, 0},
		{[]byte{0xFF, 0xFF}, 0, 0, 2},
		{[]byte{0x00, 0x12, 0x00}, 1, 1, 1},
		{[]byte{0x00, 0x00, 0x34}, 2, 0, 1},
	}
	for _, c := range cases {
		left, right := trimZeros(c.row)
		if left != c.left || right != c.right {
			t.Errorf("trimZeros(% X) = %d,%d want %d,%d", c.row, left, right, c.left, c.right)
		}
		if len(c.row)-left-right != c.tail {
			t.Errorf("trimZeros(% X) 数据长度 = %d want %d", c.row, len(c.row)-left-right, c.tail)
		}
	}
}

func TestFrameHeader(t *testing.T) {
	// 官方 SDK：3A | 宽(mm) | 高低 | 高高 | 长度低中高 | 点位码(200=5, 300=6)
	got := frameHeader(11, 8, 2, Point200)
	want := []byte{0x3A, 0x01, 0x02, 0x00, 0x0B, 0x00, 0x00, 0x05}
	if !bytes.Equal(got, want) {
		t.Errorf("frameHeader = % X, want % X", got, want)
	}

	got = frameHeader(0x010203, 384, 0x0102, Point300)
	want = []byte{0x3A, 32, 0x02, 0x01, 0x03, 0x02, 0x01, 0x06}
	if !bytes.Equal(got, want) {
		t.Errorf("frameHeader 300 点 = % X, want % X", got, want)
	}
}

func TestPointGeometry(t *testing.T) {
	if Point200.DotsPerMM() != 8 || Point200.MaxWidthDots() != 384 {
		t.Errorf("200 点参数错误: %d dots/mm, %d dots", Point200.DotsPerMM(), Point200.MaxWidthDots())
	}
	if Point300.DotsPerMM() != 12 || Point300.MaxWidthDots() != 576 {
		t.Errorf("300 点参数错误: %d dots/mm, %d dots", Point300.DotsPerMM(), Point300.MaxWidthDots())
	}
}

func TestSettingsCommand(t *testing.T) {
	s := Settings{Darkness: 8, Speed: 3, PaperType: 2, PaperFeed: 1, Close: 4, Language: 1, Sound: 1}
	got := SettingsCommand(s, false)
	want := []byte{0x3A, 0x5A, 0x83, 0x21, 0x41, 0x10, 0x00, 0xDA}
	if !bytes.Equal(got, want) {
		t.Errorf("SettingsCommand = % X, want % X", got, want)
	}
	if tail := SettingsCommand(s, true)[7]; tail != 0xCA {
		t.Errorf("临时参数尾字节 = %#x, want CA", tail)
	}
}
