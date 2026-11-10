# puqu-print

璞趣 PUQU 蓝牙标签打印机的 macOS 命令行工具。

官方只提供 Android / iOS / Windows / 小程序 / Web 客户端，Web 版的蓝牙打印还依赖 Windows
端的 `PUQUPrintService.exe`，**macOS 没有任何官方支持**。本项目直接用 CoreBluetooth
复刻了官方 SDK 的 BLE 协议，让 Mac 也能扫描、连接并打印标签。

## 支持的设备

- 璞趣 AQ 系列（实测：`AQ-V26xxxxxxx(BLE)`，热敏、384 点/行、48mm、蓝牙+USB）
- 协议与官方 SDK 的 `puqu-print` 一致，理论上覆盖 PQ / AQ / TQ / Q1 系列里走同一套
  BLE 协议的机型

## 构建

需要 macOS + Go 1.24 以上（依赖 cgo 调用 CoreBluetooth，首次运行会弹蓝牙授权）。

```bash
go build -o puqu-print .
```

## 用法

```bash
# 扫描附近的 BLE 设备
./puqu-print scan -timeout 10s

# 连接打印机，打印连接信息并读取设备回包
./puqu-print info

# 打印一张标签（PNG/JPEG）
./puqu-print print label.png

# 40mm 窄标签纸：内容居中，两侧各留 4mm 空白
./puqu-print print label.png -paper-mm 40

# 300 点机型
./puqu-print print label.png -point 300 -paper-mm 40
```

常用参数：

| 参数 | 说明 |
| --- | --- |
| `-name` | 设备名关键字，默认 `AQ-` |
| `-timeout` | 扫描超时，默认 `15s` |
| `-point` | 设备点位，`200`（默认）或 `300` |
| `-paper-mm` | 标签纸宽度（毫米），`0` 表示用满打印头 |
| `-width` | 内容最大点阵宽度，优先级高于 `-paper-mm` |
| `-align` | 内容对齐方式：`center`（默认）/ `left` / `right` |

### 排版规则

打印头固定是满幅宽（200 点为 384 点 / 48mm，300 点为 576 点），窄标签纸靠留白定位：

- 图片**比可用宽度窄时保持原始点阵尺寸，不放大**，多余的在两侧留白
- 图片**比可用宽度宽时等比缩小**，不裁切
- 打印帧头里的「宽度(mm)」始终按打印头满幅填写，与发放的行宽保持一致

例如 40mm 标签纸：`-paper-mm 40` → 内容 320 点，两侧各留 32 点（4mm）。

> 打印前请确认打印机已开机、装好标签纸，并且没有被手机 App 占用连接。

## 协议

BLE 层没有公开文档，以下格式从官方小程序 SDK
（`sdk_puquprint.7z` → `MiniProgram/.../bluetooth-printer-sdk`）的实现中还原，
并在真机上验证发送链路。

### GATT

| 用途 | 首选 | 备选 |
| --- | --- | --- |
| 服务 | `AE30` | `AE3A` / `FFE0` / `18F0` / `FE01` / `FF00` |
| 写特征 | `AE01` | `FFE1` |
| 通知特征 | `AE02` | `FFE2` |

分包：默认 20 字节一包，包间隔 10ms；有 MTU 时按 `clamp(MTU-3, 20, 244)` 取包长。

### 打印帧

帧头 8 字节：

```
0x3A | 宽度(mm) | 高度低 | 高度高 | 长度低 | 长度中 | 长度高 | 点位码
```

点位码 `5` = 200 点（8 点/mm），`6` = 300 点（12 点/mm）。帧头发完等 3ms 再发数据体。

数据体按行组织，每段 5 字节头 + 数据：

```
0x3C | 重复行数低 | 重复行数高 | 左侧零字节数 | 右侧零字节数 | 数据...
```

- 每行是 1bit/像素、MSB 优先打包（行宽按 8 点补齐）
- 连续相同的行合并成一个段，用重复行数表达
- 整行为空时按 SDK 约定写成 `左侧零=0、右侧零=行宽`
- 二值化阈值：亮度 < 200 视为黑点

### 控制指令

| 指令 | 字节 |
| --- | --- |
| 读设备详情 | `3A 5A 00 00 00 00 00 5A` |
| 读设备状态 | `3A 5A 00 00 00 00 00 0A` |
| 取消打印 | `18 1B 40` |

设备参数写入（浓度/速度/纸张/进纸/关机/语言/声音）见 `internal/puqu/protocol.go`
的 `SettingsCommand`。

## 代码结构

| 文件 | 作用 |
| --- | --- |
| `main.go` | CLI 入口：`scan` / `info` / `print` |
| `internal/puqu/transport.go` | 扫描、连接、GATT 绑定、分包写入、通知回调 |
| `internal/puqu/protocol.go` | 帧封装、行游程压缩、控制指令、设备参数 |
| `internal/puqu/bitmap.go` | 图片缩放、二值化、1bit 打包、居中留白 |
| `internal/puqu/image.go` | PNG/JPEG 读取 |
| `internal/puqu/packet.go` | 设备详情/状态回包解析 |

## 已知限制

- 只实现了位图打印和读取设备回包，`info` 目前把设备回包按原始十六进制打印，
  字段解码（电量、点位、告警码）尚未实现
- 未实现自动连接、后台保活和打印队列
- 设备休眠唤醒（`3A 5A 00 00 00 00 00 3A`）指令已记录但未接入

## 参考

- 璞趣官网：<https://www.puqulabel.com>
- SDK 下载：<https://appsres.puqulabel.com/softs/sdk_puquprint.7z>
