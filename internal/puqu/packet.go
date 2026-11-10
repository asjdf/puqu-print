package puqu

// 设备回包的解析规则取自官方 SDK 的通知处理函数。
// 所有回包都以 0x3A 开头，用第 7 字节（或详情包的尾字节）区分类型。

const (
	packetMagic     = 0x3A
	packetStateType = 0x0A
	packetDetailTyp = 0x5A
)

// Details 是设备详情回包（浓度/速度/纸张/电量等）。
type Details struct {
	Darkness  int // 1-11
	Speed     int // 1-5
	PaperType int // 1 连续纸 2 间隙纸 3 黑标纸 4 黑隙纸
	PaperFeed int // 1 开启 2 关闭
	Close     int // 1-7 自动关机
	Language  int // 1 简体 2 繁体 3 英语
	Sound     int // 1 开启 2 关闭

	BatteryLevel int // 0-15
	Warning      int
	Point        Point

	Raw []byte
}

// State 是设备状态回包。
type State struct {
	LackOfPaper    bool
	BoxCoverOpened bool
	AbnormalData   bool
	LowPower       bool
	Printing       bool
	PrintError     bool
	CancelPrinting bool

	CacheSize      int
	ProcessedPages int
	WarningCode    int

	Raw []byte
}

// ParseDetails 解析设备详情回包；不匹配时返回 nil, false。
func ParseDetails(b []byte) (*Details, bool) {
	if len(b) < 8 || b[0] != packetMagic {
		return nil, false
	}
	if b[7] != packetDetailTyp && !(len(b) > 15 && b[15] == packetDetailTyp) {
		return nil, false
	}
	point := Point200
	if 2 == (0x0F & b[7]) {
		point = Point300
	}
	return &Details{
		Darkness:     1 + int(b[3]>>4&0x0F),
		Speed:        1 + int(b[3]&0x0F),
		PaperType:    1 + int(b[4]>>4&0x0F),
		PaperFeed:    1 + int(b[4]&0x0F),
		Close:        1 + int(b[5]>>4&0x0F),
		Language:     1 + int(b[5]&0x0F),
		Sound:        1 + int(b[6]>>4&0x0F),
		BatteryLevel: int(b[6] & 0x0F),
		Warning:      int(b[7] >> 4 & 0x0F),
		Point:        point,
		Raw:          b,
	}, true
}

// ParseState 解析设备状态回包；不匹配时返回 nil, false。
func ParseState(b []byte) (*State, bool) {
	if len(b) < 8 || b[0] != packetMagic || b[7] != packetStateType {
		return nil, false
	}
	flags := b[1]
	state := &State{
		LackOfPaper:    flags&0x80 != 0,
		BoxCoverOpened: flags&0x40 != 0,
		AbnormalData:   flags&0x20 != 0,
		LowPower:       flags&0x10 != 0,
		Printing:       flags&0x08 != 0,
		PrintError:     flags&0x04 != 0,
		CancelPrinting: flags&0x02 != 0,
		CacheSize:      int(b[4]),
		ProcessedPages: int(b[3]),
		WarningCode:    int(b[6] >> 4 & 0x0F),
		Raw:            b,
	}
	if state.ready() {
		state.ProcessedPages = 0
	}
	return state, true
}

// ready 与 SDK 的 isReady 一致：无异常、未在打印、也不在等待。
func (s *State) ready() bool {
	abnormal := s.LackOfPaper || s.BoxCoverOpened || s.AbnormalData ||
		s.LowPower || s.PrintError || s.CancelPrinting
	return !abnormal && !s.Printing
}

// NormalPrint 表示设备处于可打印状态。
func (s *State) NormalPrint() bool {
	return !(s.LackOfPaper || s.BoxCoverOpened || s.AbnormalData ||
		s.LowPower || s.PrintError || s.CancelPrinting)
}

// Ready 表示设备空闲且可打印。
func (s *State) Ready() bool { return s.NormalPrint() && !s.Printing }

// Issues 返回人类可读的异常描述，正常时为空。
func (s *State) Issues() []string {
	var out []string
	if s.LackOfPaper {
		out = append(out, "缺纸")
	}
	if s.BoxCoverOpened {
		out = append(out, "纸仓打开")
	}
	if s.AbnormalData {
		out = append(out, "数据异常")
	}
	if s.LowPower {
		out = append(out, "电量低")
	}
	if s.PrintError {
		out = append(out, "打印错误")
	}
	if s.CancelPrinting {
		out = append(out, "正在取消")
	}
	if s.WarningCode != 0 {
		out = append(out, "告警码 "+itoa(s.WarningCode))
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
