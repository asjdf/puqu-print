package puqu

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func solidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestPackImageDoesNotUpscale(t *testing.T) {
	img := solidImage(8, 2, color.Black)
	rows, w, h := PackImage(img, 64)
	if w != 8 || h != 2 {
		t.Fatalf("尺寸 = %dx%d, want 8x2（不应放大）", w, h)
	}
	for y := range rows {
		if !bytes.Equal(rows[y], []byte{0xFF}) {
			t.Errorf("第 %d 行 = % X, want FF", y, rows[y])
		}
	}
}

func TestPackImageDownscales(t *testing.T) {
	img := solidImage(64, 32, color.White)
	_, w, h := PackImage(img, 32)
	if w != 32 || h != 16 {
		t.Errorf("尺寸 = %dx%d, want 32x16", w, h)
	}
}

func TestPackImageAlphaIsWhite(t *testing.T) {
	// 全透明图片不应产生黑点。
	img := image.NewRGBA(image.Rect(0, 0, 8, 1))
	rows, _, _ := PackImage(img, 8)
	if !bytes.Equal(rows[0], []byte{0x00}) {
		t.Errorf("透明像素应为白: % X", rows[0])
	}
}

func TestPackImageThreshold(t *testing.T) {
	// 亮度 199 -> 黑点；亮度 201 -> 白点。
	img := image.NewRGBA(image.Rect(0, 0, 8, 1))
	img.Set(0, 0, color.Gray{Y: 199})
	img.Set(1, 0, color.Gray{Y: 201})
	rows, _, _ := PackImage(img, 8)
	if rows[0][0] != 0x80 {
		t.Errorf("阈值判定错误: % X, want 80", rows[0][0])
	}
}
