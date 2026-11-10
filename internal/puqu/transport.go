// Package puqu 实现璞趣(PUQU)标签打印机的 BLE 传输与打印协议。
package puqu

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

// 官方 SDK 里声明的服务/特征候选表，按优先级排列。
var (
	serviceCandidates = []string{"AE30", "AE3A", "FFE0", "18F0", "FE01", "FF00"}
	writeCandidates   = []string{"AE01", "FFE1"}
	notifyCandidates  = []string{"AE02", "FFE2"}
)

// DefaultNameKeyword 是璞趣 AQ 系列设备广播名的公共前缀。
const DefaultNameKeyword = "AQ-"

// DeviceInfo 描述一次扫描到的设备。
type DeviceInfo struct {
	Name    string
	Address string
	RSSI    int
	adv     bluetooth.ScanResult
}

// Scan 扫描 timeout 时长，把命中的设备通过 onFound 回调抛出（可能多次）。
func Scan(keyword string, timeout time.Duration, onFound func(DeviceInfo)) error {
	if err := adapter.Enable(); err != nil {
		return fmt.Errorf("启用蓝牙失败: %w", err)
	}

	var (
		stopOnce sync.Once
		stopped  = make(chan struct{})
	)
	stop := func() {
		stopOnce.Do(func() {
			defer close(stopped)
			// Scan 会阻塞在内部 channel 上，StopScan 的发送需要它先就位。
			for i := 0; i < 200; i++ {
				if adapter.StopScan() == nil {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}

	go func() {
		_ = adapter.Scan(func(a *bluetooth.Adapter, r bluetooth.ScanResult) {
			name := deviceName(r)
			if keyword != "" && !strings.Contains(strings.ToUpper(name), strings.ToUpper(keyword)) {
				return
			}
			onFound(DeviceInfo{Name: name, Address: r.Address.String(), RSSI: int(r.RSSI), adv: r})
		})
	}()

	select {
	case <-time.After(timeout):
	case <-stopped:
	}
	stop()
	return nil
}

// Find 扫描并返回第一个名字命中 keyword 的设备。
func Find(keyword string, timeout time.Duration) (DeviceInfo, error) {
	if keyword == "" {
		keyword = DefaultNameKeyword
	}
	found := make(chan DeviceInfo, 1)
	var once sync.Once
	if err := Scan(keyword, timeout, func(d DeviceInfo) {
		once.Do(func() { found <- d })
	}); err != nil {
		return DeviceInfo{}, err
	}
	select {
	case d := <-found:
		return d, nil
	default:
		return DeviceInfo{}, fmt.Errorf("%s 内未找到名称包含 %q 的打印机", timeout, keyword)
	}
}

// Printer 是一条已建立的打印机连接。
type Printer struct {
	Info DeviceInfo

	dev     bluetooth.Device
	writeC  bluetooth.DeviceCharacteristic
	notifyC bluetooth.DeviceCharacteristic
	chunk   int

	mu       sync.Mutex
	onNotify func([]byte)
}

// Connect 连接指定设备并准备好可写/可通知特征。
func Connect(info DeviceInfo) (*Printer, error) {
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("启用蓝牙失败: %w", err)
	}
	dev, err := adapter.Connect(info.adv.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	p := &Printer{Info: info, dev: dev, chunk: 20}
	if err := p.bind(); err != nil {
		_ = dev.Disconnect()
		return nil, err
	}
	return p, nil
}

// Open 扫描 + 连接一步到位。
func Open(keyword string, timeout time.Duration) (*Printer, error) {
	info, err := Find(keyword, timeout)
	if err != nil {
		return nil, err
	}
	return Connect(info)
}

// Close 断开连接。
func (p *Printer) Close() error {
	return p.dev.Disconnect()
}

func (p *Printer) bind() error {
	services, err := p.dev.DiscoverServices(nil)
	if err != nil {
		return fmt.Errorf("发现服务失败: %w", err)
	}

	byService := map[string][]bluetooth.DeviceCharacteristic{}
	var order []string
	for _, s := range services {
		key := shortUUID(s.UUID().String())
		chars, err := s.DiscoverCharacteristics(nil)
		if err != nil {
			continue
		}
		byService[key] = chars
		order = append(order, key)
	}

	pick := func(want []string) (bluetooth.DeviceCharacteristic, bool) {
		for _, svc := range serviceCandidates {
			if !contains(order, svc) {
				continue
			}
			for _, name := range want {
				for _, c := range byService[svc] {
					if shortUUID(c.UUID().String()) == name {
						return c, true
					}
				}
			}
		}
		// 兜底：任何一个服务里匹配名字。
		for _, svc := range order {
			for _, name := range want {
				for _, c := range byService[svc] {
					if shortUUID(c.UUID().String()) == name {
						return c, true
					}
				}
			}
		}
		return bluetooth.DeviceCharacteristic{}, false
	}

	w, ok := pick(writeCandidates)
	if !ok {
		return fmt.Errorf("未找到可写特征，服务: %v", order)
	}
	p.writeC = w

	if n, ok := pick(notifyCandidates); ok {
		p.notifyC = n
		if err := n.EnableNotifications(p.dispatch); err != nil {
			return fmt.Errorf("订阅通知失败: %w", err)
		}
	}
	if mtu, err := w.GetMTU(); err == nil && mtu > 23 {
		p.chunk = clamp(int(mtu)-3, 20, 244)
	}
	return nil
}

func (p *Printer) dispatch(buf []byte) {
	p.mu.Lock()
	cb := p.onNotify
	p.mu.Unlock()
	if cb != nil {
		data := make([]byte, len(buf))
		copy(data, buf)
		cb(data)
	}
}

// SetNotifyHandler 注册通知回调，用于接收设备回包。
func (p *Printer) SetNotifyHandler(cb func([]byte)) {
	p.mu.Lock()
	p.onNotify = cb
	p.mu.Unlock()
}

// NotifySupported 表示设备是否提供通知特征。
func (p *Printer) NotifySupported() bool { return p.notifyC.UUID().String() != "" }

// write 按官方 SDK 的节拍分包发送：默认 20 字节一包，包间隔 10ms。
func (p *Printer) write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	for off := 0; off < len(data); off += p.chunk {
		end := off + p.chunk
		if end > len(data) {
			end = len(data)
		}
		if _, err := p.writeC.Write(data[off:end]); err != nil {
			return fmt.Errorf("写入失败(偏移 %d): %w", off, err)
		}
		if end < len(data) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return nil
}

// Status 打印当前连接的服务/特征，便于排查。
func (p *Printer) Status() string {
	mtu, _ := p.writeC.GetMTU()
	return fmt.Sprintf("设备 %s (%s)\n可写特征 %s  通知特征 %s  MTU=%d  分包=%d",
		p.Info.Name, p.Info.Address,
		shortUUID(p.writeC.UUID().String()),
		orNone(shortUUID(p.notifyC.UUID().String())),
		mtu, p.chunk)
}

func deviceName(r bluetooth.ScanResult) string {
	if n := r.LocalName(); n != "" {
		return n
	}
	return r.AdvertisementPayload.LocalName()
}

func shortUUID(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	// 标准蓝牙 128 位基址 → 取 16 位短名。
	if len(s) == 36 && strings.HasSuffix(s, "-0000-1000-8000-00805F9B34FB") {
		return strings.TrimPrefix(strings.SplitN(s, "-", 2)[0], "0000")
	}
	return s
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func orNone(s string) string {
	if s == "" {
		return "(无)"
	}
	return s
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ErrNotConnected 表示打印机不可用。
var ErrNotConnected = errors.New("打印机未连接")
