// puqu-print 是璞趣(PUQU)蓝牙标签打印机的 macOS 命令行工具。
//
// 官方只提供 Android/iOS/Windows 客户端，本工具直接复刻了官方 SDK 的 BLE
// 协议，让 macOS 也能打印标签。
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/asjdf/puqu-print/internal/puqu"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "scan":
		err = cmdScan(os.Args[2:])
	case "info":
		err = cmdInfo(os.Args[2:])
	case "print":
		err = cmdPrint(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `puqu-print - 璞趣(PUQU)蓝牙标签打印机命令行工具

用法:
  puqu-print scan  [选项]            扫描附近的 BLE 设备
  puqu-print info  [选项]            连接打印机，列出连接信息并读取设备回包
  puqu-print print <图片> [选项]      把 PNG/JPEG 打印到标签

选项:
  -name string    设备名关键字 (默认 "AQ-")
  -timeout duration
                  扫描超时 (默认 15s)
  -point int      设备点位，200 或 300 (默认 200)
  -paper-mm float 标签纸宽度(毫米)，0 = 用满打印头
  -width int      内容最大点阵宽度；优先级高于 -paper-mm
  -align string   内容对齐 center|left|right (默认 center)

图片比可用宽度窄时保持原始尺寸、两侧留白，不会被放大；更宽则等比缩小。

示例:
  puqu-print print label.png -paper-mm 40
  puqu-print print label.png -point 300 -paper-mm 40 -align left
`)
}

func cmdScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	keyword := fs.String("name", "", "设备名关键字，留空列出全部")
	timeout := fs.Duration("timeout", 10*time.Second, "扫描时长")
	fs.Parse(args)

	fmt.Printf("扫描 %s...\n", *timeout)
	return puqu.Scan(*keyword, *timeout, func(d puqu.DeviceInfo) {
		fmt.Printf("%-34s %s  RSSI=%d\n", d.Name, d.Address, d.RSSI)
	})
}

func cmdInfo(args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	keyword := fs.String("name", puqu.DefaultNameKeyword, "设备名关键字")
	timeout := fs.Duration("timeout", 15*time.Second, "扫描超时")
	fs.Parse(args)

	p, err := puqu.Open(*keyword, *timeout)
	if err != nil {
		return err
	}
	defer p.Close()

	fmt.Println(p.Status())
	if !p.NotifySupported() {
		fmt.Println("该设备未提供通知特征，读不到回包")
		return nil
	}

	got := make(chan []byte, 16)
	p.SetNotifyHandler(func(b []byte) { got <- b })

	fmt.Println("请求设备详情与状态...")
	if err := p.ReadDetails(); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	if err := p.ReadState(); err != nil {
		return err
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case b := <-got:
			fmt.Printf("回包 %2d 字节: %s\n", len(b), puqu.Hex(b))
			reportPacket(b)
		case <-deadline:
			return nil
		}
	}
}

func reportPacket(b []byte) {
	if d, ok := puqu.ParseDetails(b); ok {
		fmt.Printf("  设备详情: 浓度 %d  速度 %d  纸张 %s  进纸 %s  自动关机 %s  语言 %s  声音 %s  电量 %d  点位 %d\n",
			d.Darkness, d.Speed, paperType(d.PaperType), onOff(d.PaperFeed),
			closeTime(d.Close), language(d.Language), onOff(d.Sound), d.BatteryLevel, d.Point)
		fmt.Printf("  提示: 本机点位为 %d，打印时用 -point %d\n", d.Point, d.Point)
	}
	if s, ok := puqu.ParseState(b); ok {
		fmt.Printf("  设备状态: 缓存 %d  已处理 %d  打印中 %v  可打印 %v\n",
			s.CacheSize, s.ProcessedPages, s.Printing, s.Ready())
		if issues := s.Issues(); len(issues) > 0 {
			fmt.Println("  异常:", strings.Join(issues, "、"))
		}
	}
}

func paperType(v int) string {
	switch v {
	case 1:
		return "连续纸"
	case 2:
		return "间隙纸"
	case 3:
		return "黑标纸"
	case 4:
		return "黑隙纸"
	}
	return fmt.Sprintf("%d", v)
}

func onOff(v int) string {
	if v == 1 {
		return "开"
	}
	return "关"
}

func closeTime(v int) string {
	names := []string{"5分钟", "15分钟", "30分钟", "1小时", "2小时", "6小时", "永不"}
	if v >= 1 && v <= len(names) {
		return names[v-1]
	}
	return fmt.Sprintf("%d", v)
}

func language(v int) string {
	switch v {
	case 1:
		return "简体"
	case 2:
		return "繁体"
	case 3:
		return "英语"
	}
	return fmt.Sprintf("%d", v)
}

func cmdPrint(args []string) error {
	fs := flag.NewFlagSet("print", flag.ExitOnError)
	keyword := fs.String("name", puqu.DefaultNameKeyword, "设备名关键字")
	timeout := fs.Duration("timeout", 15*time.Second, "扫描超时")
	width := fs.Int("width", 0, "点阵宽度，0 = 按点位取满宽")
	pointN := fs.Int("point", 200, "设备点位，200 或 300")
	paperMM := fs.Float64("paper-mm", 0, "标签纸宽度(毫米)，0 = 满打印头")
	align := fs.String("align", "center", "内容对齐 center|left|right")
	fs.Parse(reorderArgs(args))

	if fs.NArg() < 1 {
		return fmt.Errorf("缺少图片路径，例如: puqu-print print label.png")
	}
	point := puqu.Point(*pointN)
	if point != puqu.Point200 && point != puqu.Point300 {
		return fmt.Errorf("点位只能是 200 或 300，收到 %d", *pointN)
	}

	head := point.MaxWidthDots()
	content := *width
	switch {
	case content > 0:
	case *paperMM > 0:
		content = int(*paperMM*float64(point.DotsPerMM()) + 0.5)
	default:
		content = head
	}
	if content > head {
		return fmt.Errorf("内容宽度 %d 点超过打印头 %d 点", content, head)
	}
	if rem := content % 8; rem != 0 {
		content += 8 - rem
	}

	rows, content, height, err := puqu.LoadImageRows(fs.Arg(0), content)
	if err != nil {
		return err
	}
	offset, err := alignOffset(*align, head, content)
	if err != nil {
		return err
	}
	rows = puqu.PlaceRows(rows, head, offset)
	fmt.Printf("内容 %d x %d 点，两侧留白 %d/%d 点，整行 %d 点，共 %d 行，点位 %d\n",
		content, height, offset, head-content-offset, head, len(rows), point)

	p, err := puqu.Open(*keyword, *timeout)
	if err != nil {
		return err
	}
	defer p.Close()
	fmt.Println(p.Status())

	start := time.Now()
	if err := p.Print(rows, head, point); err != nil {
		return err
	}
	fmt.Printf("已发送，用时 %s\n", time.Since(start).Round(time.Millisecond))
	return nil
}

func alignOffset(align string, head, content int) (int, error) {
	switch align {
	case "center", "":
		return (head - content) / 2, nil
	case "left":
		return 0, nil
	case "right":
		return head - content, nil
	}
	return 0, fmt.Errorf("对齐方式只能是 center/left/right，收到 %q", align)
}

// reorderArgs 把选项挪到位置参数前面。
// Go 的 flag 包遇到第一个非选项参数就停止解析，要支持
// `print a.png -paper-mm 40` 这种写法就得先把选项挑出来。
// 本命令的选项都带值，成对搬运即可。
func reorderArgs(args []string) []string {
	var opts, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			continue
		}
		opts = append(opts, a)
		if !strings.Contains(a, "=") && i+1 < len(args) {
			opts = append(opts, args[i+1])
			i++
		}
	}
	return append(opts, positional...)
}
