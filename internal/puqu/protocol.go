package puqu

import (
	"encoding/hex"
	"time"
)

// 打印机点位。200 点 = 8 点/mm（384 点 ≈ 48mm），300 点 = 12 点/mm。
type Point int

const (
	Point200 Point = 200
	Point300 Point = 300
)

// DotsPerMM 返回每毫米点数，官方 SDK 用它把像素宽换算成毫米写进帧头。
func (p Point) DotsPerMM() int {
	if p == Point300 {
		return 12
	}
	return 8
}

// MaxWidthDots 返回单行最大点阵宽度。
func (p Point) MaxWidthDots() int {
	if p == Point300 {
		return 576
	}
	return 384
}

func (p Point) code() byte {
	if p == Point300 {
		return 6
	}
	return 5
}

// 控制指令，取自官方 SDK 实现。
var (
	CmdReadDetails = []byte{0x3A, 0x5A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x5A}
	CmdReadState   = []byte{0x3A, 0x5A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0A}
	CmdWake        = []byte{0x3A, 0x5A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3A}
	CmdCancelPrint = []byte{0x18, 0x1B, 0x40}
)

// Settings 对应 SDK 的 DeviceDetails，全部保存在设备侧。
type Settings struct {
	Darkness  int // 1-11
	Speed     int // 1-5
	PaperType int // 1 连续纸 2 间隙纸 3 黑标纸 4 黑隙纸
	PaperFeed int // 1 开启 2 关闭
	Close     int // 1-7 自动关机
	Language  int // 1 简体 2 繁体 3 英语
	Sound     int // 1 开启 2 关闭
}

// SettingsCommand 生成写入设备参数的指令；temporary 对应 SDK 的临时参数。
func SettingsCommand(s Settings, temporary bool) []byte {
	tail := byte(0xDA)
	if temporary {
		tail = 0xCA
	}
	return []byte{
		0x3A, 0x5A,
		byte(s.Darkness<<4 | s.Speed),
		byte(s.PaperType<<4 | s.PaperFeed),
		byte(s.Close<<4 | s.Language),
		byte(s.Sound << 4),
		0x00, tail,
	}
}

// frameHeader 构造打印帧头：
// 3A | 宽(mm) | 高低 | 高高 | 长度低 | 长度中 | 长度高 | 点位码
func frameHeader(payloadLen, widthDots, heightDots int, point Point) []byte {
	return []byte{
		0x3A,
		byte(widthDots / point.DotsPerMM()),
		byte(heightDots & 0xFF),
		byte(heightDots >> 8 & 0xFF),
		byte(payloadLen & 0xFF),
		byte(payloadLen >> 8 & 0xFF),
		byte(payloadLen >> 16 & 0xFF),
		point.code(),
	}
}

// Print 发送一页位图。rows 为每行已按 MSB 优先打包的 1bit 数据。
func (p *Printer) Print(rows [][]byte, widthDots int, point Point) error {
	payload := compressRows(rows)
	height := len(rows)

	if err := p.write(frameHeader(len(payload), widthDots, height, point)); err != nil {
		return err
	}
	time.Sleep(3 * time.Millisecond)
	return p.write(payload)
}

// ReadDetails 请求设备详情，回包走通知。
func (p *Printer) ReadDetails() error { return p.write(CmdReadDetails) }

// ReadState 请求设备状态，回包走通知。
func (p *Printer) ReadState() error { return p.write(CmdReadState) }

// CancelPrint 取消当前打印任务。
func (p *Printer) CancelPrint() error { return p.write(CmdCancelPrint) }

// compressRows 按官方 SDK 的格式打包：相同行合并计数，
// 每段为 3C | 计数低 | 计数高 | 左侧零字节数 | 右侧零字节数 | 数据。
func compressRows(rows [][]byte) []byte {
	out := make([]byte, 0, len(rows)*16)
	for i := 0; i < len(rows); {
		n := rowRun(rows, i)
		out = appendRow(out, rows[i], n)
		i += n
	}
	return out
}

// rowRun 返回从 start 起连续相同的行数。
func rowRun(rows [][]byte, start int) int {
	n := 1
	for start+n < len(rows) && equalRow(rows[start], rows[start+n]) {
		n++
	}
	return n
}

func equalRow(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func appendRow(out, row []byte, count int) []byte {
	left, right := trimZeros(row)
	out = append(out,
		0x3C,
		byte(count&0xFF),
		byte(count>>8&0xFF),
		byte(left),
		byte(right),
	)
	return append(out, row[left:len(row)-right]...)
}

// trimZeros 返回行首/行尾全零字节数量；整行全零时按 SDK 约定返回 0 / 整行。
func trimZeros(row []byte) (int, int) {
	left, right := 0, 0
	for left < len(row) && row[left] == 0 {
		left++
	}
	for right < len(row)-left && row[len(row)-1-right] == 0 {
		right++
	}
	if left+right >= len(row) {
		return 0, len(row)
	}
	return left, right
}

// Hex 便于调试打印原始字节。
func Hex(b []byte) string { return hex.EncodeToString(b) }
