// puqu-print 是璞趣(PUQU)蓝牙标签打印机的 macOS 命令行工具。
//
// 官方只提供 Android/iOS/Windows 客户端，本工具直接复刻了官方 SDK 的 BLE
// 协议，让 macOS 也能打印标签。
package main

import (
	"flag"
	"fmt"
	"os"
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
  -width int      点阵宽度，0 表示按点位取满宽 (200 点 -> 384, 300 点 -> 576)
  -point int      设备点位，200 或 300 (默认 200)

示例:
  puqu-print print label.png
  puqu-print print label.png -point 300 -width 576
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
		case <-deadline:
			return nil
		}
	}
}

func cmdPrint(args []string) error {
	fs := flag.NewFlagSet("print", flag.ExitOnError)
	keyword := fs.String("name", puqu.DefaultNameKeyword, "设备名关键字")
	timeout := fs.Duration("timeout", 15*time.Second, "扫描超时")
	width := fs.Int("width", 0, "点阵宽度，0 = 按点位取满宽")
	pointN := fs.Int("point", 200, "设备点位，200 或 300")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("缺少图片路径，例如: puqu-print print label.png")
	}
	point := puqu.Point(*pointN)
	if point != puqu.Point200 && point != puqu.Point300 {
		return fmt.Errorf("点位只能是 200 或 300，收到 %d", *pointN)
	}
	w := *width
	if w <= 0 {
		w = point.MaxWidthDots()
	}

	rows, height, err := puqu.LoadImageRows(fs.Arg(0), w)
	if err != nil {
		return err
	}
	fmt.Printf("位图 %d x %d 点，共 %d 行，点位 %d\n", w, height, len(rows), point)

	p, err := puqu.Open(*keyword, *timeout)
	if err != nil {
		return err
	}
	defer p.Close()
	fmt.Println(p.Status())

	start := time.Now()
	if err := p.Print(rows, w, point); err != nil {
		return err
	}
	fmt.Printf("已发送，用时 %s\n", time.Since(start).Round(time.Millisecond))
	return nil
}
