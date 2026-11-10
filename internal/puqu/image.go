package puqu

import (
	"fmt"
	"image"
	"os"
)

// LoadImageRows 读取 PNG/JPEG 并转成打印行数据。
// maxWidth 是内容的最大点阵宽度：超过会等比缩小，不足则保持原尺寸（不放大）。
func LoadImageRows(path string, maxWidth int) (rows [][]byte, widthDots, heightDots int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()

	if maxWidth <= 0 {
		return nil, 0, 0, fmt.Errorf("宽度必须为正，收到 %d", maxWidth)
	}
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("解码图片失败(%s): %w", path, err)
	}
	_ = format
	rows, widthDots, heightDots = PackImage(img, maxWidth)
	return rows, widthDots, heightDots, nil
}
