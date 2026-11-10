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

// enableAdapter 保证整个进程只初始化一次 BLE 栈。
//
// Windows 后端把 Enable 实现成 ole.RoInitialize，同一线程重复调用返回 S_FALSE(1)，
// 而 go-ole 把任何非 0 返回值都当错误，于是「先扫描再连接」的流程会随机报
// "启用蓝牙失败: 函数不正确"。这里做一次去重，失败不缓存、可重试。
var (
	enableMu   sync.Mutex
	enableDone bool
)

func enableAdapter() error {
	enableMu.Lock()
	defer enableMu.Unlock()
	if enableDone {
		return nil
	}
	if err := adapter.Enable(); err != nil {
		return fmt.Errorf("启用蓝牙失败: %w", err)
	}
	enableDone = true
	return nil
}

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
	addr    bluetooth.Address
}

// FindByAddress 跳过扫描，直接用 MAC 地址构造一个可连接的设备。
// 适合已知地址、或扫描后连接不稳定的场景（Windows 上更稳）。
func FindByAddress(s string) (DeviceInfo, error) {
	addr, err := ParseAddress(s)
	if err != nil {
		return DeviceInfo{}, err
	}
	return DeviceInfo{Name: addr.String(), Address: addr.String(), addr: addr}, nil
}

// Scan 扫描 timeout 时长，把命中的设备通过 onFound 回调抛出（可能多次）。
func Scan(keyword string, timeout time.Duration, onFound func(DeviceInfo)) error {
	if err := enableAdapter(); err != nil {
		return err
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
		seen := map[string]bool{}
		_ = adapter.Scan(func(a *bluetooth.Adapter, r bluetooth.ScanResult) {
			name := deviceName(r)
			if keyword != "" && !strings.Contains(strings.ToUpper(name), strings.ToUpper(keyword)) {
				return
			}
			// 同一个设备在一次扫描里会被反复上报（广播包 + 扫描回包），列表里只留第一条。
			key := r.Address.String()
			if seen[key] {
				return
			}
			seen[key] = true
			onFound(DeviceInfo{Name: name, Address: key, RSSI: int(r.RSSI), addr: r.Address})
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

// ConnectOptions 控制连接行为。
type ConnectOptions struct {
	// Timeout 是单次连接的超时时间；Windows 上建立 GATT 会话偏慢，默认给足 20s。
	Timeout time.Duration
	// Retries 是失败后的重试次数（不含首次尝试）。
	Retries int
	// OnProgress 非空时会在每次连接尝试前后回调，便于排查连接问题。
	OnProgress func(msg string)
}

func (o ConnectOptions) timeout() time.Duration {
	if o.Timeout <= 0 {
		return 20 * time.Second
	}
	return o.Timeout
}

func (o ConnectOptions) retries() int {
	if o.Retries < 0 {
		return 0
	}
	return o.Retries
}

// connTimeout 把期望超时换算成 tinygo 的 ConnectionTimeout。
// 该字段是 uint16、步进 0.625ms，直接传大于 40.96s 的值会溢出回绕成很短的时间，
// 所以这里显式钳到上限，并返回实际生效的时长。
func connTimeout(want time.Duration) (bluetooth.Duration, time.Duration) {
	const unit = 625 * time.Microsecond
	units := int64(want / unit)
	if units < 1 {
		units = 1
	}
	if units > 65535 {
		units = 65535
	}
	return bluetooth.Duration(units), time.Duration(units) * unit
}

func (o ConnectOptions) progress(msg string, args ...any) {
	if o.OnProgress != nil {
		o.OnProgress(fmt.Sprintf(msg, args...))
	}
}

// Connect 连接指定设备并准备好可写/可通知特征。
func Connect(info DeviceInfo, opts ConnectOptions) (*Printer, error) {
	if err := enableAdapter(); err != nil {
		return nil, err
	}
	ct, effective := connTimeout(opts.timeout())
	params := bluetooth.ConnectionParams{ConnectionTimeout: ct}

	var lastErr error
	total := opts.retries() + 1
	for attempt := 1; attempt <= total; attempt++ {
		if attempt > 1 {
			time.Sleep(500 * time.Millisecond)
		}
		opts.progress("正在连接 %s (%s)，第 %d/%d 次尝试，单次超时 %s...",
			info.Name, info.Address, attempt, total, effective.Round(10*time.Millisecond))
		start := time.Now()
		dev, err := adapter.Connect(info.addr, params)
		if err != nil {
			opts.progress("第 %d/%d 次连接失败（耗时 %s）: %v", attempt, total, time.Since(start).Round(time.Millisecond), err)
			lastErr = fmt.Errorf("连接 %s (%s) 第 %d/%d 次尝试失败（耗时 %s）: %w",
				info.Name, info.Address, attempt, total, time.Since(start).Round(time.Millisecond), err)
			continue
		}
		opts.progress("已连接（耗时 %s），正在发现服务...", time.Since(start).Round(time.Millisecond))
		p := &Printer{Info: info, dev: dev, chunk: 20}
		if err := p.bind(); err != nil {
			_ = dev.Disconnect()
			lastErr = err
			continue
		}
		return p, nil
	}
	return nil, lastErr
}

// Open 扫描 + 连接一步到位。
func Open(keyword string, timeout time.Duration, opts ConnectOptions) (*Printer, error) {
	info, err := Find(keyword, timeout)
	if err != nil {
		return nil, err
	}
	return Connect(info, opts)
}

// OpenByAddress 跳过扫描，直接按 MAC 地址连接。
func OpenByAddress(addr string, opts ConnectOptions) (*Printer, error) {
	info, err := FindByAddress(addr)
	if err != nil {
		return nil, err
	}
	return Connect(info, opts)
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
