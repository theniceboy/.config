#!/usr/bin/env bash
# System power graph for tmux status-right.
# Samples whole-system draw (mW) every call (SMC PSTR via smc-pstr, ~1 Hz,
# same source as iStat Menus) into a ring buffer and renders a packed braille
# area chart (one dot per reading, bottom-filled; bands
# 10/18/25/30/35 W, or 10/20/30/40/50 W when connected). ioreg telemetry is
# only a fallback and the AC state is refreshed every AC_REFRESH sec, so the
# expensive calls stay off the per-second path. Output is cached between
# samples so it is safe to call more often than SAMPLE_SEC. Runs concurrently
# (one instance per tmux client): no set -e, all writes atomic, and stdout is
# never empty or partial or the status segment would flash.
set -o pipefail

DIR="$HOME/.cache/tmux-power"
BINDIR="$(cd "$(dirname "$0")" && pwd)"
LOG="$DIR/samples.log"
RENDER="$DIR/render"
STATE="$DIR/last"
ACSTATE="$DIR/ac"
TRIMSTATE="$DIR/trim"
LOCK="$DIR/lock"
SAMPLE_SEC="${POWER_GRAPH_INTERVAL:-1}"
AC_REFRESH=30
TRIM_REFRESH=60
CHARS="${POWER_GRAPH_CHARS:-15}"
MAXKEEP=12000

mkdir -p "$DIR"
now=$(date +%s)

# One sampler per tick: concurrent clients race past a whole-second timestamp
# gate and would double-log (graph scrolls 2 dots/s). Holders killed mid-run
# leave the lock; it is taken back after LOCK_STALE_SEC.
have_lock=0
if mkdir "$LOCK" 2>/dev/null; then
  have_lock=1
else
  lm=$(stat -f %m "$LOCK" 2>/dev/null || echo "$now")
  if (( now - lm > 10 )); then
    rm -rf "$LOCK" 2>/dev/null
    mkdir "$LOCK" 2>/dev/null && have_lock=1
  fi
fi

if (( have_lock )); then
trap 'rmdir "$LOCK" 2>/dev/null' EXIT
last=$(cat "$STATE" 2>/dev/null || echo 0)
[[ $last =~ ^[0-9]+$ ]] || last=0

if (( now - last >= SAMPLE_SEC )); then
  echo "$now" > "$STATE"

  w="" src=""
  if pm=$("$BINDIR/smc-pstr" 2>/dev/null); then
    w=$(awk -v p="$pm" 'BEGIN{printf "%d", p*1000}')
    src=p
  fi

  ac_ts=0 ac_val=""
  ac_line=$(cat "$ACSTATE" 2>/dev/null || true)
  read -r ac_ts ac_val <<<"$ac_line" || true
  [[ $ac_ts =~ ^[0-9]+$ ]] || ac_ts=0

  need_ioreg=0
  [[ -z $src ]] && need_ioreg=1
  (( now - ac_ts >= AC_REFRESH )) && need_ioreg=1

  if (( need_ioreg )); then
    out=$(ioreg -rn AppleSmartBattery 2>/dev/null || true)

    if [[ -z $src ]]; then
      w=$(grep -oE '"SystemLoad"=[0-9]+' <<<"$out" | head -1 | cut -d= -f2 || true)
      src=a
      if [[ ! $w =~ ^[0-9]+$ ]] || (( w == 0 )); then
        amp=$(grep -oE '"InstantAmperage"=[0-9]+' <<<"$out" | head -1 | cut -d= -f2 || true)
        volt=$(grep -oE '"AppleRawBatteryVoltage"=[0-9]+' <<<"$out" | head -1 | cut -d= -f2 || true)
        [[ -n ${volt:-} ]] || volt=$(grep -oE '"Voltage"=[0-9]+' <<<"$out" | head -1 | cut -d= -f2 || true)
        if [[ ${amp:-} =~ ^[0-9]+$ && ${volt:-} =~ ^[0-9]+$ ]] && (( volt > 0 )); then
          w=$(awk -v a="$amp" -v v="$volt" 'BEGIN{if(a>9223372036854775807)a-=18446744073709551616; if(a<0)a=-a; printf "%d", a*v/1000}')
          src=b
        fi
      fi
    fi

    if (( now - ac_ts >= AC_REFRESH )); then
      # Connected = AC with an adapter rated >=15 W, matching kitty power-mode.zsh.
      ac=0
      aw=$(grep -oE '"Watts"=[0-9]+' <<<"$out" | head -1 | cut -d= -f2 || true)
      if grep -qE '"ExternalConnected" *= *Yes' <<<"$out" && [[ ${aw:-} =~ ^[0-9]+$ ]] && (( aw >= 15 )); then
        ac=1
      fi
      printf '%s %s\n' "$now" "$ac" > "$ACSTATE"
      ac_val=$ac
    fi
  fi

  if [[ $w =~ ^[0-9]+$ ]] && (( w > 0 )); then
    printf '%s %s %s\n' "$now" "$w" "$src" >> "$LOG"
    trim_ts=$(cat "$TRIMSTATE" 2>/dev/null || echo 0)
    [[ $trim_ts =~ ^[0-9]+$ ]] || trim_ts=0
    if (( now - trim_ts >= TRIM_REFRESH )); then
      echo "$now" > "$TRIMSTATE"
      tail -n "$MAXKEEP" "$LOG" > "$LOG.tmp.$$" 2>/dev/null && mv -f "$LOG.tmp.$$" "$LOG"
    fi
  fi

  ac=${ac_val:-0}
  python3 - "$LOG" "$CHARS" "$ac" > "$RENDER.tmp.$$" <<'PYEOF' || true
import sys
log, chars, ac = sys.argv[1], int(sys.argv[2]), sys.argv[3] == "1"
ws = []
try:
    for l in open(log):
        try:
            ws.append(int(l.split()[1]) / 1000)
        except (ValueError, IndexError):
            continue
except OSError:
    pass
ws = ws[-2 * chars:]
if not ws:
    sys.exit(0)
cols = ws
BOUNDS = [10, 20, 30, 40, 50] if ac else [10, 18, 25, 30, 35]
def bucket(w):
    for i, b in enumerate(BOUNDS):
        if w < b:
            return i
    return 5
LB = [0x40, 0x04, 0x02, 0x01]
RB = [0x80, 0x20, 0x10, 0x08]
parts = []
for i in range(0, len(cols) - 1, 2):
    l, r = bucket(cols[i]), bucket(cols[i + 1])
    ch = chr(0x2800 + sum(LB[:min(l, 4)]) + sum(RB[:min(r, 4)]))
    if max(l, r) == 5:
        parts.append("#[fg=colour196]" + ch + "#[fg=colour248]")
    else:
        parts.append(ch)
icon = "⚡" if ac else "🔋"
sys.stdout.write(
    f"#[fg=colour117,bg=#232530] {icon}{ws[-1]:.0f}W #[fg=colour248]{''.join(parts)}"
    f"#[fg=colour117] ~{sum(ws)/len(ws):.0f} ↑{max(ws):.0f} #[default]"
)
PYEOF
  [[ -s "$RENDER.tmp.$$" ]] && mv -f "$RENDER.tmp.$$" "$RENDER"
  rm -f "$RENDER.tmp.$$" "$LOG.tmp.$$" 2>/dev/null
fi
fi

[[ -s $RENDER ]] && cat "$RENDER"
exit 0
