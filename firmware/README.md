# Slate / Firmware

ESP-IDF 5.5 固件，目标芯片 ESP32-S3。目前只支持 **ZecTrix Note4 V1.0**（极趣实验室「Ai 便利贴」）：4.2 英寸黑白墨水屏、ES8311 音频、MEMS 麦克风、3 个按键、单节锂电池。

本目录是独立的 ESP-IDF 工程。

## 构建与烧录

```bash
source $IDF_PATH/export.sh
idf.py -C firmware build
idf.py -C firmware -p <serial> flash monitor
```

- target、Flash、PSRAM、分区表都写在 `sdkconfig.defaults` 里，不需要 `set-target`。
- CI 使用 v5.5.6，产物为 `slate-full.bin`（合并后的完整镜像）和 `slate-ota.bin`（只含应用）。
- 只更新应用、保留配网和绑定信息时用 `idf.py app-flash`，它不写 NVS。

## 代码结构

| 目录 | 内容 |
| --- | --- |
| `main/app/` | 生命周期编排 |
| `main/startup/` | 启动模式判定、首次注册流程 |
| `main/bsp/` | 板级 GPIO、电源、ADC、充电状态 |
| `main/drivers/` | EPD（SSD1683 兼容）、ES8311 音频、I2C 总线、按键 |
| `main/network/` | Wi-Fi、SNTP、配网页（SoftAP + DNS 劫持）、凭据存储 |
| `main/sync/` | 后端 API 客户端与同步服务 |
| `main/storage/` | LittleFS 缓存、NVS |
| `main/scenes/` | 界面场景：Splash、Frame、BgRefresh、Settings、Xiaozhi |
| `main/ui/` | 状态栏、帧视图、菜单 |
| `main/events/` | 事件总线与 UI 事件 |
| `main/power/` | 电源状态、休眠管理、关机、分钟时钟 |
| `main/xiaozhi/` | 小智：配置激活、协议（MQTT / WebSocket）、MCP、对话服务 |
| `main/resources/` | 配网页 HTML、内置字体 |
| `main/utils/` | JSON、字节、GPIO、MAC 工具 |

---

## 硬件

| 项 | 规格 |
| --- | --- |
| MCU | ESP32-S3-WROOM-1 N16R8V：16 MB QIO Flash + 8 MB Octal PSRAM |
| 显示 | 4.2" 黑白 EPD，400×300，SSD2683（命令兼容 SSD1683） |
| 音频 | ES8311 codec、单声道扬声器、MEMS 麦克风、差分 D 类功放 |
| 其他 | PCF8563 RTC、GT23SC6699 NFC、USB-C（CDC/JTAG） |
| 按键 | 确认 / BOOT、上、下 / 开机，另有 EN 硬复位 |

Flash 为 QIO、PSRAM 为 Octal，所以 `CONFIG_ESPTOOLPY_OCT_FLASH=n` 配合 `CONFIG_SPIRAM_MODE_OCT=y`。GPIO 26–37 被 PSRAM 占用。

### GPIO

| 引脚 | 网络名 | 说明 |
| --- | --- | --- |
| GPIO0 | `KEY_ENTER` | 确认 / BOOT，低有效 |
| GPIO1 | `STDBY_H` | 充电 IC 充满状态 |
| GPIO2 | `CHRG_L` | 充电 IC 充电状态 |
| GPIO3 | `LED_G` | 绿灯，低有效 |
| GPIO4 | `ADC_BAT` | VBAT 1:2 分压（ADC1 CH3） |
| GPIO5 | `RTC_INT` | PCF8563 INT# |
| GPIO6 | `EPD_PWR_EN` | EPD 供电 |
| GPIO7 | `NFC_FD` | NFC 场检测 |
| GPIO8 | `EPD_BUSY` | 低 = 忙，高 = 空闲 |
| GPIO9 | `EPD_NRES` | EPD 复位 |
| GPIO10 | `EPD_NDC` | EPD 数据 / 命令选择 |
| GPIO11 | `EPD_NCS` | EPD 片选，软件控制 |
| GPIO12 | `EPD_SCK` | SPI 时钟 |
| GPIO13 | `EPD_SDA` | SPI MOSI |
| GPIO14 | `I2S_MCLK` | I2S 主时钟 |
| GPIO15 | `I2S_SCLK` | I2S 位时钟 |
| GPIO16 | `I2S_ASDOUT` | 麦克风数据 |
| GPIO17 | `PWR_ON` | 主电源自锁，高 = 保持供电 |
| GPIO18 | `KEY_DET` | 下键 / 开机 |
| GPIO19 | `USB_DN` | USB D− |
| GPIO20 | `USB_DP` | USB D+ |
| GPIO21 | `NFC_PWR` | NFC 供电 |
| GPIO38 | `I2S_LRCK` | I2S 帧时钟 |
| GPIO39 | `KEY_PGUP` | 上键；不是 RTC IO，不能唤醒深睡 |
| GPIO42 | `PA_PWR_EN` | AVDD_3V3：音频供电 + I2C 上拉 |
| GPIO43 | `TXD0` | UART0 TX |
| GPIO44 | `RXD0` | UART0 RX |
| GPIO45 | `I2S_DSDIN` | 扬声器数据 |
| GPIO46 | `PA_CTRL` | 功放使能，高 = 出声 |
| GPIO47 | `I2C_SDA` | I2C 数据 |
| GPIO48 | `I2C_SCL` | I2C 时钟 |

### 总线

| 总线 | 引脚 | 设备 |
| --- | --- | --- |
| I2C0 | SDA=47、SCL=48 | ES8311 0x18、PCF8563 0x51、GT23SC6699 0x55 |
| SPI3 | SCK=12、MOSI=13、CS=11、DC=10、RST=9、BUSY=8 | EPD，40 MHz，mode 0 |
| I2S0 | MCLK=14、BCLK=15、WS=38、DIN=16、DOUT=45 | ES8311 全双工 |

### 电源

按住下键（GPIO18）会拉低 PMOS 栅极接通主电源，固件随后把 GPIO17 拉高自锁，松手也不会断电。开机时要等 GPIO18 松开再交给按键驱动，否则开机那一下会被识别成按键。

| 供电 | 控制 | 注意 |
| --- | --- | --- |
| 主电源 | GPIO17 | 拉低即整机断电；深睡前必须用 RTC GPIO hold 保持高电平，否则按键无法唤醒 |
| EPD | GPIO6 | 关闭后画面保留，但控制器状态丢失，重新上电需完整初始化 |
| AVDD_3V3 | GPIO42 | 音频与 I2C 上拉；关闭后所有 I2C 操作都会失败 |

### 分区

| 分区 | 偏移 | 大小 | 用途 |
| --- | --- | --- | --- |
| `nvs` | `0x9000` | 24 KB | Wi-Fi、服务端地址、设备密钥、小智配置 |
| `phy_init` | `0xf000` | 4 KB | 射频校准数据 |
| `factory` | `0x10000` | 4 MB | 应用固件 |
| `storage` | `0x410000` | 约 12 MB | LittleFS，缓存帧与音频 |

---

## 启动

启动模式由 `boot_mode::Decide()` 决定：

| 模式 | 条件 | 行为 |
| --- | --- | --- |
| Portal | 没有 Wi-Fi 凭据 | 开启配网热点 |
| BackgroundRefresh | 定时器唤醒，且已注册、有缓存 | 不点亮界面，同步当前组，当前帧变化时才刷屏，然后继续深睡 |
| FullActive | 其他情况 | 显示界面、联网同步 |

### 配网

- 热点名为 `Slate-XXXX`：前缀可配置，后缀取 MAC 后两字节。所有 DNS 请求都指向 `192.168.4.1`。
- 配网页分两步填写 Wi-Fi 和服务端地址。
- 固件先试连 Wi-Fi，成功后才保存并重启。

### 注册

1. 联网并完成 SNTP 对时（HTTPS 需要正确时间）。
2. 如果 NVS 里还没有设备密钥（`dev_sec`），调用 `POST /api/device/register` 注册。
3. 屏幕显示配对码，等待在 Web 端绑定。

之后的请求都带 `Authorization: Bearer <设备密钥>`。连续收到 5 次 401 时清除密钥并重启，重新注册。

---

## 同步

同步服务运行在 `slate_sync` task 中，接口定义见 [backend/README.md](../backend/README.md#设备协议-v2)。

### 轮询

| 状态 | 间隔 |
| --- | --- |
| 已绑定 | 60 秒 |
| 未绑定，前 10 分钟 | 10 秒 |
| 未绑定，10–30 分钟 | 30 秒 |
| 未绑定，30 分钟后 | 60 秒 |

- 每次 `sync` / `group` 请求都带上本地缓存的 `manifestVersion`。版本一致时服务端不返回 manifest，设备不下载任何内容。
- 版本变化时响应里直接带新 manifest。设备按 sha256 只下载缺失或变化的图片和音频（`GET /api/device/blobs/{hash}`），并校验大小和哈希。
- 服务端返回 426 表示固件协议版本过旧，需要升级固件。

### 定时刷新

- 动态帧带 `refreshAt`（Unix 秒）。深睡前按 `refreshAt - 当前时间` 设置 RTC 定时器，最短 60 秒。
- 连续联系不上服务器时按指数退避，最长 1 小时。
- 定时器唤醒时上报当前帧的 `currentContentId`。服务端会先重新渲染这一帧（如果已到期），设备在同一次唤醒中拿到新图。
- 只有当前帧的图片或状态栏变化时才刷新屏幕。

### 缓存写入

整组同步分两步，保证任何时候失败都不会留下半份缓存：

1. **下载**：缺失的文件先下载到该组的暂存目录。这一步失败不影响已有缓存。
2. **提交**：全部下载完成后逐帧替换（每帧的图片、音频和 meta 一起换），最后才写 manifest 版本。提交中途失败或断电时版本号保持旧值，下一轮同步会拿到完整 manifest，按 hash 补齐不一致的帧。

缓存最多保留 4 个组，并保证至少 1 MB 空闲空间。下载前按待下载体积先清理其他组；目标组本身放不下时同步报错。

### 屏幕待重绘

提交前设备会标记「屏幕待重绘」，确认物理屏幕已显示新缓存后才清除：

- 前台模式：提交成功即清除，界面用真实的上一帧缓冲重绘。
- 后台刷新：等 EPD 刷新完成才清除；刷新超时或撞到截止时间则保留。

标记存在时，下一轮成功同步按内容已变化处理。后台刷新不拿缓存当旧画面做局刷，而是全刷当前帧。

### 缓存布局

| 路径 | 内容 |
| --- | --- |
| `/littlefs/layout` | 缓存布局版本，不一致时启动时整体格式化 |
| `/littlefs/state.json` | 所选组、manifest 版本、当前帧位置、访问计数 |
| `/littlefs/groups/{gid}/manifest.json` | 组名、manifest 版本、帧数、最近访问序号 |
| `/littlefs/groups/{gid}/frames/{idx}.img` / `.pcm` / `.meta` | 帧的图片、音频和元数据，idx 为帧在 manifest 中的位置 |

---

## 按键与界面

| 操作 | 行为 |
| --- | --- |
| 上 / 下 短按 | 上一帧 / 下一帧 |
| 上 / 下 长按 | 上一个 / 下一个内容组 |
| 确认 短按 | 下一帧 |
| 确认 长按 | 设置：音量调节、设备信息、重启设备、恢复出厂 |
| 确认 双击 | 小智语音对话 |
| 上 + 下 同时按 | 全屏刷新，清除残影 |

所有界面操作都在 `ui_loop` task 中执行，其他 task 通过 FreeRTOS 队列投递事件。`UiEvent` 必须可以按值拷贝，不能包含 `std::string` 或其他持有内存的容器。

## 显示

- 1bpp 帧为 15000 字节；SSD2683 按 2bpp 传输，SPI 数据量是 30 KB。
- 全刷约 2–3 秒，局刷约 0.3–0.6 秒；连续局刷 8 次后做一次全刷清残影。
- 温度补偿读数缓存 60 秒。
- 深睡前只等待进行中的刷新完成，不额外刷屏，画面靠墨水屏自身保持。

## 音频

I2S0 全双工，16 kHz、单声道、16-bit，MCLK = 256 × fs。

- 发送方向播放内容音频和小智回复，接收方向用于小智录音。
- 内容音频格式与后端一致（raw s16le PCM），与小智共用同一个音量设置。
- ES8311 在第一次播放时才打开；打开后等 100 ms 再开功放，切换音频前先静音，以减少爆音。

## 休眠

闲置 `SLATE_IDLE_DEEP_SLEEP_MIN`（默认 3）分钟后进入深睡。

**不进入深睡的情况**

- 配网模式
- 插着 USB 或正在充电
- 小智对话中
- 设备未绑定的前 2 小时（便于在 Web 端绑定，低电量时例外）

**唤醒源**：确认键（GPIO0）、下键（GPIO18）、插入 USB（GPIO2）。当前帧是需要定时刷新的动态帧时，额外开启 RTC 定时唤醒。

**进入顺序**：停止小智、同步、音频 → 等待屏幕刷新完成 → 关闭 EPD 和 AVDD 供电 → hold 住 GPIO17 → 配置唤醒源。

---

## 配置项

在 [main/Kconfig.projbuild](main/Kconfig.projbuild) 中：

| 项 | 默认 | 说明 |
| --- | --- | --- |
| `SLATE_DEFAULT_SERVER_URL` | 空 | 配网页里预填的服务端地址 |
| `SLATE_AP_SSID_PREFIX` | `Slate` | 热点名前缀 |
| `SLATE_DEFAULT_TIMEZONE` | `CST-8` | POSIX 时区 |
| `SLATE_IDLE_DEEP_SLEEP_MIN` | `3` | 闲置多少分钟后深睡 |

组件依赖见 [main/idf_component.yml](main/idf_component.yml)。内置字体由仓库根目录的 `scripts/fonts/firmware.sh` 生成。

## 调试提示

- 服务端地址可以是 `http://`，但鉴权请求走明文时会打印警告。
- 设备协议版本是 `api::kProtocol`（`sync/api_client.h`），要和后端 `api.DeviceProtocol` 一致。
- 调试 EPD 时注意：BUSY 是低电平表示忙，和 SSD1683 手册的默认极性相反。
