package puqu

import (
	"image"
	"image/color"
	"image/draw"

	xdraw "golang.org/x/image/draw"

	_ "image/jpeg"
	_ "image/png"
)

// Threshold 是灰度二值化阈值，与官方 SDK 保持一致（亮度 < 200 视为黑点）。
const Threshold = 200

// PackImage 把任意图片按目标点阵宽度等比缩放，再转成 MSB 优先的 1bit 行数据。
// 返回的行宽固定为 widthDots/8 字节。scale 为缩放插值质量。
func PackImage(img image.Image, widthDots int) (rows [][]byte, height int) {
	if widthDots%8 != 0 {
		widthDots += 8 - widthDots%8
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	height = sh * widthDots / sw
	if height < 1 {
		height = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, widthDots, height))
	// 白底，避免透明像素被当成黑点。
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)

	rows = make([][]byte, height)
	for y := 0; y < height; y++ {
		row := make([]byte, widthDots/8)
		for x := 0; x < widthDots; x++ {
			r, g, bl, a := dst.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			lum := (299*int(r>>8) + 587*int(g>>8) + 114*int(bl>>8) + 500) / 1000
			if lum < Threshold {
				row[x/8] |= 1 << (7 - uint(x%8))
			}
		}
		rows[y] = row
	}
	return rows, height
}

// PackRows 把已二值化的位图（每字节表示 8 个像素的行）直接转换成 1bit 行数据，
// 用于文本/条码等自绘内容。
func PackRows(bits []bool, widthDots, heightDots int) ([][]byte, int) {
	if widthDots%8 != 0 {
		widthDots += 8 - widthDots%8
	}
	rows := make([][]byte, heightDots)
	for y := 0; y < heightDots; y++ {
		row := make([]byte, widthDots/8)
		for x := 0; x < widthDots; x++ {
			if y*widthDots+x >= len(bits) {
				continue
			}
			if bits[y*widthDots+x] {
				row[x/8] |= 1 << (7 - uint(x%8))
			}
		}
		rows[y] = row
	}
	return rows, heightDots
}
