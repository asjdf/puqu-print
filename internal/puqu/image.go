package puqu

import (
	"fmt"
	"image"
	"os"
)

// LoadImageRows 读取 PNG/JPEG 并按 widthDots 宽等比缩放成打印行数据。
func LoadImageRows(path string, widthDots int) ([][]byte, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	if widthDots <= 0 {
		return nil, 0, fmt.Errorf("宽度必须为正，收到 %d", widthDots)
	}
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, 0, fmt.Errorf("解码图片失败(%s): %w", path, err)
	}
	_ = format
	rows, height := PackImage(img, widthDots)
	return rows, height, nil
}
