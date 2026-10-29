# Orange Pi Zero3 板级指南

> 本指南面向在 **Orange Pi Zero3（Allwinner H618）** 上部署 PicoClaw 的用户，覆盖硬件规格、
> 系统准备、安装、配置、运维与排障。所有板级信息已对照官方用户手册 v1.1 与 Linux 主线
> 设备树核实。

## 1. 硬件规格（核实版）

| 项目 | 规格 |
|---|---|
| SoC | Allwinner H618，四核 Cortex-A53 @ 最高 1.5GHz |
| GPU | Mali G31 MP2（⚠️ Linux 下无闭源驱动加速，仅靠 framebuffer 显示） |
| 内存 | 1GB / 1.5GB / 2GB / 4GB LPDDR4（与 GPU 共享） |
| 存储 | microSD 卡槽 + 板载 16MB SPI Flash（出厂微 Linux，仅供恢复） |
| 网络 | 千兆以太网（YT8531C PHY）；WiFi 5 + BT 5.0（**AW859A**，a/b/g/n/ac） |
| 视频 | Micro HDMI 2.0a；TV CVBS 输出（经 13pin 扩展板） |
| USB | 1× USB2.0 Host 板载 + 2× USB2.0 Host（经 13pin 扩展板）；Type-C 仅供电 |
| 音频 | 板载 codec 耳机输出（**经 13pin 扩展板 3.5mm 口**）；HDMI 音频；⚠️ **无板载麦克风** |
| 调试串口 | 3 针 UART（TXD/RXD/GND，115200 8N1） |
| LED | 设备树暴露两颗：`green:power`（绿灯，上电常亮，留给内核）与 `red:status`（红灯，**本适配接管**输出灯语）。Armbian 6.18.x 实测慢闪的是红灯 |
| 看门狗 | 硬件看门狗 `/dev/watchdog` |
| 温度传感器 | 4 个热区：cpu / gpu / ddr / ve（`/sys/class/thermal/thermal_zone*/temp`，毫摄氏度） |
| 供电 | Type-C 5V/2A（或 3A），26pin/13pin 的 5V 引脚也可供电 |
| 尺寸 | 85mm × 56mm，30g |

### 26pin 引脚表（17 路 GPIO，3.3V 电平）

| 引脚 | 功能 | | 引脚 | 功能 |
|---|---|---|---|---|
| 1 | 3.3V | | 2 | 5V |
| 3 | PH5 (TWI3-SDA / I2C) | | 4 | 5V |
| 5 | PH4 (TWI3-SCK / I2C) | | 6 | GND |
| 7 | PC9 | | 8 | PH2 (UART5_TX) |
| 9 | GND | | 10 | PH3 (UART5_RX) |
| 11 | PC6 | | 12 | PC11 |
| 13 | PC5 | | 14 | GND |
| 15 | PC8 | | 16 | PC15 |
| 17 | 3.3V | | 18 | PC14 |
| 19 | PH7 (SPI1_MOSI) | | 20 | GND |
| 21 | PH8 (SPI1_MISO) | | 22 | PC7 |
| 23 | PH6 (SPI1_CLK) | | 24 | PH9 (SPI1_CS) |
| 25 | GND | | 26 | PC10 |

> 26pin 自带 I2C×1（TWI3）、SPI×1（SPI1）、UART×1（UART5）+ 多路 GPIO；PWM 亦可用。
> 完整官方说明见《Orange Pi Zero3 用户手册》3.16/3.18 节。

### 13pin 扩展板接口

| 引脚 | 功能 |
|---|---|
| 1 | 5V |
| 2 | GND |
| 3/4 | USB2-DM / USB2-DP（USB2.0 Host 第 2 路） |
| 5/6 | USB3-DM / USB3-DP（USB2.0 Host 第 3 路） |
| 7/8 | LINEOUT R / L（耳机左右声道） |
| 9 | TV-OUT |
| 10 | PC1（不接扩展板时可作 GPIO） |
| 11 | PI16（同上） |
| 12 | PI6（同上） |
| 13 | PH10 / IR-RX（红外接收，需扩展板） |

> ⚠️ 13pin 扩展板上的 **MIC 在 Zero3 上不可用**（H618 板卡未引出麦克风输入）。
> 如需语音输入请外接 USB 声卡/麦克风；语音输出可用 13pin 扩展板耳机口
> （`aplay -D hw:0,0 test.wav`，card 0 为板载 codec）或 HDMI。

## 2. 系统准备

### 2.1 镜像选择（推荐顺序）

1. **Armbian**（bookworm 主线内核，社区维护最好，本适配首选）
2. **官方 Debian 12 Bookworm / Ubuntu 22.04**（kernel 6.1，驱动完整：LED/GPIO/I2C/SPI/UART/PWM/温度/看门狗均 OK）

烧录：balenaEtcher / Win32DiskImager / `dd if=xxx.img of=/dev/sdX bs=4M conv=fsync`。

### 2.2 首次启动

- **有线**：插网线即可，DHCP 自动获取 IP（路由器里查，或 `arp -a`）。
- **WiFi（AW859A）**：`sudo nmtui` 图形化连网（或 `nmcli dev wifi connect <SSID> password <密码>`）。
- **无显示器**：用 3 针调试串口（USB-TTL 模块，115200 8N1，TXD↔RXD 交叉）。
- **系统更新**：`sudo apt update && sudo apt upgrade -y`。

### 2.3 供电与散热

- 使用 5V/2A 以上电源；外接 USB 设备时建议 5V/3A。
- H618 满载温度较高，建议贴散热片（可加 5V 风扇，接 26pin 的 5V/GND）。
- 安装脚本会把调频策略设为 `schedutil`（负载低时自动降频省电）。

## 3. 安装

### 3.1 用 Gitea Release 产物（推荐）

每次推送到 main 都会自动发布**滚动 Nightly Release**（预发布版），产物含 arm64/amd64
二进制与 SHA256SUMS 校验文件。仓库是**私有仓库**，下载需要 Gitea 凭据：

```bash
# 浏览器打开（需登录 Gitea）下载 picoclaw-opi-zero3-linux-arm64:
#   http://192.168.31.101:3100/admin/picoclaw-opi-zero3/releases
# 或命令行直接下载（nightly 标签）:
wget --user <Gitea用户名> --password <令牌或密码> \
  http://192.168.31.101:3100/admin/picoclaw-opi-zero3/releases/download/nightly/picoclaw-opi-zero3-linux-arm64
```

### 3.2 从源码构建（x86 机器上交叉编译）

```bash
# 需要 Go 1.25.12+（golang.google.cn 或 https://go.dev/dl/）
git clone http://192.168.31.101:3100/admin/picoclaw-opi-zero3.git   # 私有仓库，需凭据
cd picoclaw-opi-zero3
make opi-zero3 opi-info        # 产出 build/*-linux-arm64
```

### 3.3 一键安装

```bash
sudo deploy/orangepi-zero3/install.sh --binary ./build/picoclaw-opi-zero3-linux-arm64
```

脚本会自动完成：

- 创建系统用户 `picoclaw`（数据目录 `/var/lib/picoclaw`）
- 安装 `/usr/local/bin/picoclaw` 与 `opi-info`
- 生成 `/var/lib/picoclaw/.picoclaw/config.json`（模板见 `deploy/orangepi-zero3/`）
- 安装 `picoclaw.service`（内存上限 400M/600M、`Restart=on-failure` 自动重启；
  系统 systemd ≥253 且存在 `/dev/watchdog` 时另由 `/etc/systemd/system.conf.d/` drop-in 启用硬件看门狗）
- udev 规则：把状态灯（`*:status`，实测即红灯）的 `brightness/trigger` 授权给 `picoclaw` 用户（否则灯语不生效）
- ≤2GB 内存板卡启用 512MB ZRAM 交换；journald 改易失存储（保护 SD 卡）
- CPU 调频策略切到 `schedutil`

## 4. 配置

编辑 `/var/lib/picoclaw/.picoclaw/config.json`，核心是 `model_list`。模板已内置
**自建 New-API 网关**（OpenAI 兼容）示例：

```json
{
  "model_name": "deepseek-v4-flash",
  "model": "openai/DeepSeek-V4-Flash",
  "api_keys": ["sk-你的令牌"],
  "api_base": "http://192.168.31.101:3001/v1"
}
```

- `model` 前缀 `openai/` 表示 OpenAI 兼容协议，任何兼容网关（New-API/One-API/vLLM/Ollama）都可用。
- 多个条目 = 多模型；配合 `agents.defaults.model_name` 选择默认模型。
- Zero3 建议：`context_window` 65536、`max_tokens` 4096、`max_tool_iterations` 12
  （1GB 板卡可再调小 context_window 到 32768 以省内存）。
- `gateway.host = "0.0.0.0"` 对外开放网关 HTTP 端口 18790，提供健康检查与渠道 webhook：
  `http://<开发板IP>:18790/health`、`/ready`（POST `/reload` 热重载）。
  注意：该构建**不含 Web 控制台**，访问 `/` 返回 404；交互请用 `picoclaw agent` 或配置 `channel_list`。
- 修改后 `sudo systemctl restart picoclaw` 生效。

## 5. 运维

```bash
opi-info                          # 板卡诊断：型号/频率/温度/内存/LED/看门狗
opi-info -led-test                # LED 灯语演示（10 秒）
opi-info -thermal-loop            # 温度实时打印
sudo systemctl status picoclaw    # 服务状态
journalctl -u picoclaw -f         # 实时日志（含 opi_zero3 温度采样）
```

灯语含义见仓库根 [README.md](../README.md) 的 LED 灯语表。

- **看门狗**：服务单元**不含** `WatchdogSec`（网关不发 `sd_notify WATCHDOG=1`，设了只会被
  systemd 每 30 秒杀掉重启）；systemd v253+ 且存在 `/dev/watchdog` 时，安装脚本通过
  `/etc/systemd/system.conf.d/` drop-in 启用**管理器级** `RuntimeWatchdogSec=15`，
  由 PID 1 喂 H618 硬件看门狗（`/dev/watchdog`）——内核/用户态整体卡死会直接重启板卡。
  系统 systemd < 253 时跳过该步，此时进程假死不会被自动发现，只有 `Restart=on-failure`
  对真正退出/崩溃生效。
- **内存**：`MemoryHigh=400M` 软限流，`MemoryMax=600M` 硬限；ZRAM 提供压缩交换兜底。
- **日志**：journald 易失存储（重启即清），系统盘写入量很小，适合 SD 卡长期运行。

## 6. CI/CD（Gitea Actions）

`.gitea/workflows/ci.yml`（本仓唯一常跑的 CI，job 跑在 `amd64` runner 上）在 push 到
`main`/`master` 与 pull request 时：

1. 用仓库 Secret `CI_TOKEN` 本地克隆（`git clone --depth 1`）；runner 镜像**不自带 Go**，
   按 `go.mod` 要求的版本从国内镜像装工具链（多镜像回退 + 重试 + tar 完整性校验）；
2. `CGO_ENABLED=0` 交叉编译 `linux-arm64`（`goolm,stdjson,opi_zero3` 标签）、`linux-amd64`
   与 `opi-info-linux-arm64`，产物写进 `build/` 并用 `file`/`sha256sum` 校验；
3. 单元测试：`go test -v ./pkg/board/... ./pkg/events/... ./cmd/opi-info/...`（伪 sysfs，x86 上可跑）；
4. `go vet ./pkg/board/... ./cmd/opi-info/...` —— **硬门禁**，非 0 退出即 job 失败；
   刻意不写 `./...`（那会把上游 434K 行代码拉进来，上游/环境的问题不该由本仓门禁承担）；
5. 向提交写入 `opi-zero3-ci` 状态摘要（提交页可见，同时反映测试与 vet 结果）；
6. **仅 main 分支**：发布滚动 Nightly Release（`nightly` 标签预发布版），把
   arm64/amd64/opi-info 三个二进制与 SHA256SUMS.txt 作为附件挂到
   `http://192.168.31.101:3100/admin/picoclaw-opi-zero3/releases`
   （仓库为私有，下载需 Gitea 凭据）。

实测一次完整 run（#10197 / 2026-10-15 / commit `ee8d2a46`）：**7 分 03 秒** —— 克隆 +
装工具链约 40 秒，依赖下载 + 三个交叉编译约 5 分 12 秒，测试与 vet 约 15 秒。

> `deploy/ci/` 里的 447MB 本地构建镜像方案**已于 2026-10-26 删除**：它从未被任何 workflow 使用
> （镜像内置 Go 的前提已被"runner 镜像不自带 Go"这条实测推翻），留着只会让下一个人以为 CI 走的是这个镜像。
> 需要时从 git 历史取回（`git log --diff-filter=D -- deploy/ci`），但删它的理由仍然成立。

## 7. 常见问题

| 现象 | 处理 |
|---|---|
| LED 灯语不亮 | 确认 `PICOCLAW_OPI_LED=1`（`/etc/picoclaw/env`）；检查 udev 规则已装：`ls -l /sys/class/leds/*/brightness` 属组应为 `picoclaw`；`opi-info -led-test` 验证 |
| 找不到状态灯 | 官方镜像内核版本差异导致 LED 名不同，用 `PICOCLAW_OPI_LED_PATH=/sys/class/leds/<名>` 指定（`ls /sys/class/leds/` 查看） |
| 服务起不来 | `journalctl -u picoclaw -n 50`；多半是 config.json 语法错误或模型网关不通 |
| 内存不足 / OOM | 确认 ZRAM 已启用（`swapon`）；调小 `context_window`/`max_tokens`；关闭不需要的渠道 |
| 温度高 | 贴散热片、加风扇；`opi-info -thermal-loop` 观察；过热时 LED 双闪 + 日志告警 |
| 无法语音输入 | Zero3 板载无麦克风，请接 USB 声卡/麦克风 |
| WiFi 连不上 | AW859A 在官方 6.1 镜像与 Armbian 下即插即用；`sudo nmtui` 重连；5GHz 需路由器开启对应信道 |
| 端口 18790 打不开 | 确认 config 里 `gateway.host` 为 `0.0.0.0`；`ss -ltnp \| grep picoclaw` |

## 8. 与上游保持同步

```bash
git remote add upstream https://github.com/sipeed/picoclaw.git
git fetch upstream
git checkout main
git merge upstream/main        # 可能冲突：两处一行的钩子调用 + Makefile 第 1 行的 .PHONY
```

详见 [ADAPTATION.md](../ADAPTATION.md) 的完整变更清单（§2 列了全部 4 处上游文件改动与核实命令）。
