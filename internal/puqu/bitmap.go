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

// PackImage 把图片转成 MSB 优先的 1bit 行数据。
//
// 图片比 maxWidth 宽时等比缩小；比 maxWidth 窄时保持原始点阵尺寸、不做放大，
// 留给 PlaceRows 在两侧补白。返回实际使用的内容宽度（按 8 点对齐）。
func PackImage(img image.Image, maxWidth int) (rows [][]byte, widthDots, heightDots int) {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, 0, 0
	}

	widthDots = sw
	if maxWidth > 0 && widthDots > maxWidth {
		widthDots = maxWidth
	}
	if rem := widthDots % 8; rem != 0 {
		widthDots += 8 - rem
	}
	heightDots = (sh*widthDots + sw - 1) / sw
	if heightDots < 1 {
		heightDots = 1
	}

	src := img
	if widthDots != sw || heightDots != sh {
		dst := image.NewRGBA(image.Rect(0, 0, widthDots, heightDots))
		// 白底，避免透明像素被当成黑点。
		draw.Draw(dst, dst.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		src = dst
	}

	rows = make([][]byte, heightDots)
	for y := 0; y < heightDots; y++ {
		row := make([]byte, widthDots/8)
		for x := 0; x < widthDots; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
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
	return rows, widthDots, heightDots
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

// PlaceRows 把宽度为 len(row)*8 的行数据放到 lineWidth 宽的行里，左边缘位于 xOffset。
// 打印头固定是满幅宽（200 点为 384），窄标签纸靠这个把内容摆到纸中间。
func PlaceRows(rows [][]byte, lineWidth, xOffset int) [][]byte {
	contentWidth := 0
	if len(rows) > 0 {
		contentWidth = len(rows[0]) * 8
	}
	if xOffset < 0 {
		xOffset = 0
	}
	if xOffset+contentWidth > lineWidth {
		if contentWidth <= lineWidth {
			xOffset = lineWidth - contentWidth
		} else {
			xOffset = 0
		}
	}

	out := make([][]byte, len(rows))
	bytesPerLine := (lineWidth + 7) / 8
	for y, row := range rows {
		buf := make([]byte, bytesPerLine)
		for i, b := range row {
			if b == 0 {
				continue
			}
			for bit := 0; bit < 8; bit++ {
				if b&(1<<uint(7-bit)) == 0 {
					continue
				}
				x := xOffset + i*8 + bit
				if x < 0 || x >= lineWidth {
					continue
				}
				buf[x/8] |= 1 << uint(7-x%8)
			}
		}
		out[y] = buf
	}
	return out
}
