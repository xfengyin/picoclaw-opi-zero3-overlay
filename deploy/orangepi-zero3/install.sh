#!/usr/bin/env bash
# install.sh — Orange Pi Zero3 一键安装脚本 for PicoClaw
#
# 用法:
#   sudo ./install.sh                          # 自动查找 ./build 下的 arm64 二进制
#   sudo ./install.sh --binary /path/picoclaw  # 指定二进制
#   sudo ./install.sh --url https://.../picoclaw-linux-arm64  # 从 URL 下载
#   sudo ./install.sh --api-key sk-xxx         # 顺手写入模型网关令牌
#   sudo ./install.sh --uninstall              # 卸载
#
# 可选参数:
#   --no-zram       不配置 512MB ZRAM 交换（内存 >= 2GB 时建议关闭）
#   --zram-size 1G  自定义 ZRAM 大小
#   --no-watchdog   不启用 systemd 硬件看门狗
#   --skip-config   不生成初始 config.json
set -euo pipefail

SERVICE_USER=picoclaw
SERVICE_HOME=/var/lib/picoclaw
BIN_DIR=/usr/local/bin
BIN_NAME=picoclaw
BINARY=""
API_KEY=""
UNINSTALL=0
ENABLE_ZRAM=1
ZRAM_SIZE=""
ENABLE_WATCHDOG=1
SKIP_CONFIG=0
WATCHDOG_AVAILABLE=0

log()  { printf '\033[1;32m[opi]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[opi]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[opi]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,14p' "$0"
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --binary)    BINARY="${2:-}"; shift 2 ;;
    --url)       BINARY_URL="${2:-}"; shift 2 ;;
    --api-key)   API_KEY="${2:-}"; shift 2 ;;
    --zram-size) ZRAM_SIZE="${2:-}"; shift 2 ;;
    --no-zram)   ENABLE_ZRAM=0; shift ;;
    --no-watchdog) ENABLE_WATCHDOG=0; shift ;;
    --skip-config) SKIP_CONFIG=1; shift ;;
    --uninstall) UNINSTALL=1; shift ;;
    -h|--help)   usage ;;
    *) die "未知参数: $1 (使用 --help 查看用法)" ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

[ "$(id -u)" -eq 0 ] || die "请用 root 运行: sudo $0"

# ---------- 卸载 ----------
if [ "$UNINSTALL" -eq 1 ]; then
  log "卸载 PicoClaw..."
  systemctl disable --now picoclaw 2>/dev/null || true
  systemctl disable --now zramswap 2>/dev/null || true
  rm -f /etc/systemd/system/picoclaw.service /etc/systemd/system/zramswap.service
  rm -rf /etc/systemd/system/picoclaw.service.d
  rm -f /etc/systemd/system.conf.d/10-picoclaw-watchdog.conf
  rm -f /etc/udev/rules.d/99-picoclaw-led.rules
  rm -f /etc/systemd/journald.conf.d/picoclaw.conf
  rm -f "$BIN_DIR/$BIN_NAME" "$BIN_DIR/opi-info" /etc/picoclaw/env
  systemctl daemon-reload
  udevadm control --reload-rules 2>/dev/null || true
  log "已卸载（用户 $SERVICE_USER 与 /var/lib/picoclaw 数据目录保留，如需彻底删除请手动执行 userdel -r $SERVICE_USER）"
  exit 0
fi

# ---------- 板卡检测 ----------
BOARD_MODEL="$(tr -d '\0' </proc/device-tree/model 2>/dev/null || true)"
log "板卡型号: ${BOARD_MODEL:-未知}"
case "$BOARD_MODEL" in
  *[Zz]ero3*|*[Zz]ero\ 3*) log "检测到 Orange Pi Zero3 ✔" ;;
  *) warn "警告: 当前系统不是 Orange Pi Zero3（本脚本针对 Zero3 调优，其他板卡也可运行）" ;;
esac

MEM_KB="$(awk '/MemTotal/{print $2}' /proc/meminfo 2>/dev/null || echo 0)"
log "内存: $((MEM_KB/1024)) MB"
if [ -e /dev/watchdog ]; then WATCHDOG_AVAILABLE=1; fi

# ---------- 获取二进制 ----------
if [ -z "${BINARY:-}" ] && [ -n "${BINARY_URL:-}" ]; then
  log "下载: $BINARY_URL"
  curl -fSL --retry 3 -o /tmp/picoclaw-dl "$BINARY_URL"
  chmod +x /tmp/picoclaw-dl
  BINARY=/tmp/picoclaw-dl
fi
if [ -z "${BINARY:-}" ]; then
  for cand in "$SCRIPT_DIR/../../build/$BIN_NAME-opi-zero3-linux-arm64" \
              "$SCRIPT_DIR/../../build/$BIN_NAME-linux-arm64" \
              "$SCRIPT_DIR/build/$BIN_NAME-opi-zero3-linux-arm64"; do
    if [ -f "$cand" ]; then BINARY="$cand"; break; fi
  done
fi
if [ -z "${BINARY:-}" ] || [ ! -f "$BINARY" ]; then
  die "未找到 arm64 二进制。用法: sudo $0 --binary /path/picoclaw-opi-zero3-linux-arm64 或 --url <下载地址>"
fi

# 校验架构
BIN_ARCH="$(file -b "$BINARY" | grep -o 'aarch64' | head -1 || true)"
if [ "$BIN_ARCH" != "aarch64" ]; then
  warn "警告: 提供的二进制不是 aarch64（检测到: $(file -b "$BINARY" | cut -d, -f1)）"
fi

# ---------- 用户 ----------
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
  log "创建系统用户 $SERVICE_USER"
  useradd --system --create-home --home-dir "$SERVICE_HOME" \
          --shell /usr/sbin/nologin "$SERVICE_USER"
else
  [ -d "$SERVICE_HOME" ] || mkdir -p "$SERVICE_HOME"
fi
chown -R "$SERVICE_USER:$SERVICE_USER" "$SERVICE_HOME"

# The unit lists /var/log/picoclaw in ReadWritePaths; systemd refuses to start
# the service (226/NAMESPACE) when that path does not exist yet.
install -d -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0750 /var/log/picoclaw

# ---------- 安装二进制 ----------
install -m 0755 "$BINARY" "$BIN_DIR/$BIN_NAME"
OPI_INFO_SRC="${BINARY%/*}/opi-info-linux-arm64"
[ -f "$OPI_INFO_SRC" ] || OPI_INFO_SRC="$SCRIPT_DIR/../../build/opi-info-linux-arm64"
if [ -f "$OPI_INFO_SRC" ]; then install -m 0755 "$OPI_INFO_SRC" "$BIN_DIR/opi-info"; fi
log "已安装: $BIN_DIR/$BIN_NAME"
"$BIN_DIR/$BIN_NAME" version 2>/dev/null || true

# ---------- 环境文件 ----------
if [ ! -f /etc/picoclaw/env ]; then
  log "写入 /etc/picoclaw/env"
  mkdir -p /etc/picoclaw
  {
    echo "# PicoClaw Orange Pi Zero3 环境变量"
    echo "PICOCLAW_OPI_LED=1"
    echo "PICOCLAW_OPI_THERMAL_INTERVAL=60"
    echo "PICOCLAW_OPI_THERMAL_MAX=85"
    echo "PICOCLAW_CONFIG=/var/lib/picoclaw/.picoclaw/config.json"
  } > /etc/picoclaw/env
fi

# ---------- 配置 ----------
CONFIG_DIR="$SERVICE_HOME/.picoclaw"
if [ "$SKIP_CONFIG" -eq 0 ] && [ ! -f "$CONFIG_DIR/config.json" ]; then
  log "生成初始配置 $CONFIG_DIR/config.json"
  mkdir -p "$CONFIG_DIR" "$CONFIG_DIR/workspace"
  cp "$SCRIPT_DIR/config.opi-zero3.example.json" "$CONFIG_DIR/config.json"
  chown -R "$SERVICE_USER:$SERVICE_USER" "$CONFIG_DIR"
  if [ -n "$API_KEY" ]; then
    sed -i "s|sk-你的令牌|$API_KEY|g" "$CONFIG_DIR/config.json"
    log "已写入模型网关 API Key"
  else
    warn "提示: 稍后编辑 $CONFIG_DIR/config.json，把 sk-你的令牌 换成真实令牌"
  fi
else
  log "配置已存在或跳过配置，使用现有 config.json"
fi

# ---------- systemd 单元 ----------
log "安装 systemd 服务"
install -m 0644 "$SCRIPT_DIR/picoclaw.service" /etc/systemd/system/picoclaw.service

# 硬件看门狗：systemd v253+ 才支持 RuntimeWatchdogSec，用 drop-in 按版本启用
if [ "$ENABLE_WATCHDOG" -eq 1 ] && [ "$WATCHDOG_AVAILABLE" -eq 1 ]; then
  SYSVER="$(systemctl --version 2>/dev/null | awk 'NR==1{print $2}')"
  MAJOR="${SYSVER%%.*}"
  if [ "${MAJOR:-0}" -ge 253 ]; then
    # RuntimeWatchdogSec is a manager-level ([Manager]) setting: inside a unit
    # [Service] section systemd ignores it with "Unknown key name". Write a
    # system.conf.d drop-in so PID 1 pets /dev/watchdog itself.
    # Remove the unit-scope drop-in older versions wrote; systemd ignores that
    # key there and only logged "Unknown key name" on every reload.
    rm -f /etc/systemd/system/picoclaw.service.d/10-runtime-watchdog.conf
    rmdir /etc/systemd/system/picoclaw.service.d 2>/dev/null || true
    mkdir -p /etc/systemd/system.conf.d
    printf '[Manager]\nRuntimeWatchdogSec=15\n' > /etc/systemd/system.conf.d/10-picoclaw-watchdog.conf
    systemctl daemon-reexec 2>/dev/null || true
    log "systemd v$SYSVER ≥ 253，启用系统级硬件看门狗 RuntimeWatchdogSec=15"
  else
    warn "systemd v$SYSVER < 253，跳过硬件看门狗（服务单元不含 WatchdogSec，进程假死不会被自动发现）"
  fi
elif [ "$WATCHDOG_AVAILABLE" -eq 0 ]; then
  warn "无 /dev/watchdog，跳过硬件看门狗配置"
fi

# ---------- udev: LED 权限 ----------
log "安装 udev 规则（状态 LED 权限）"
install -m 0644 "$SCRIPT_DIR/99-picoclaw-led.rules" /etc/udev/rules.d/99-picoclaw-led.rules
udevadm control --reload-rules 2>/dev/null || true
udevadm trigger --subsystem-match=leds 2>/dev/null || true

# ---------- journald: 减少 SD 卡磨损 ----------
log "配置 journald（易失存储）"
mkdir -p /etc/systemd/journald.conf.d
install -m 0644 "$SCRIPT_DIR/journald-picoclaw.conf" /etc/systemd/journald.conf.d/picoclaw.conf
systemctl restart systemd-journald 2>/dev/null || true

# ---------- CPU 调频策略 ----------
log "配置 CPU 调频策略 (schedutil)"
for p in /sys/devices/system/cpu/cpufreq/policy*; do
  [ -f "$p/scaling_governor" ] || continue
  if grep -q schedutil "$p/scaling_available_governors" 2>/dev/null; then
    echo schedutil > "$p/scaling_governor" 2>/dev/null || true
  fi
done

# ---------- ZRAM ----------
if [ "$ENABLE_ZRAM" -eq 1 ]; then
  if [ "$MEM_KB" -ge 2097152 ]; then
    warn "内存 >= 2GB，默认跳过 ZRAM（如需启用请使用 --zram-size）"
  else
    log "配置 ZRAM 交换"
    install -m 0644 "$SCRIPT_DIR/zramswap.service" /etc/systemd/system/zramswap.service
    mkdir -p /etc/picoclaw
    echo "${ZRAM_SIZE:-512M}" > /etc/picoclaw/zram-size
    systemctl daemon-reload
    systemctl enable --now zramswap 2>/dev/null || warn "zramswap 启动失败（可忽略，系统仍可用）"
  fi
fi

# ---------- 启动 ----------
systemctl daemon-reload
systemctl enable picoclaw
if ! systemctl restart picoclaw; then
  warn "picoclaw 服务启动失败，查看日志: journalctl -u picoclaw -n 50"
  exit 1
fi

sleep 3
if systemctl is-active --quiet picoclaw; then
  log "✔ PicoClaw 已启动并常驻 (systemd)"
else
  warn "picoclaw 服务未运行，查看日志: journalctl -u picoclaw -n 50"
fi

# ---------- 汇总 ----------
log "安装完成。接下来的步骤:"
echo "  1. 编辑配置:   $CONFIG_DIR/config.json   (填入模型网关地址与令牌)"
echo "  2. 重启服务:   sudo systemctl restart picoclaw"
echo "  3. 查看状态:   sudo systemctl status picoclaw ; journalctl -u picoclaw -f"
echo "  4. 板卡诊断:   opi-info           (LED 演示: opi-info -led-test)"
echo "  5. 健康检查:   http://<开发板IP>:18790/health  (另见 /ready，POST /reload 热重载)"
if [ -f "$BIN_DIR/opi-info" ]; then
  "$BIN_DIR/opi-info"
fi
