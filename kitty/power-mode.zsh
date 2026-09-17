#!/bin/zsh

set -u

config="$HOME/.config/kitty/power-mode.conf"
minimum_ac_watts=15

apply_mode() {
    local mode="$1"
    local temp="${config}.tmp.$$"

    if [[ "$mode" == "ac" ]]; then
        cat > "$temp" <<'EOF'
cursor_trail 3
cursor_trail_decay 0.1 0.4
cursor_trail_start_threshold 2
cursor_trail_color #686a75
cursor_blink_interval -1
EOF
    else
        cat > "$temp" <<'EOF'
cursor_trail 0
cursor_blink_interval -1
EOF
    fi

    if cmp -s "$temp" "$config"; then
        rm -f "$temp"
        return
    fi

    mv "$temp" "$config"
    pkill -USR1 -x kitty 2>/dev/null || true
}

apply_current_mode() {
    local watts

    case "$(/usr/bin/pmset -g ps | /usr/bin/head -n 1)" in
        *"'AC Power'"*) ;;
        *)
            apply_mode battery
            return
            ;;
    esac

    watts=$(/usr/sbin/ioreg -arc AppleSmartBattery -w0 \
        | /usr/bin/plutil -extract '0.AdapterDetails.Watts' raw -o - - 2>/dev/null) || watts=""

    if [[ "$watts" == <-> ]] && (( watts >= minimum_ac_watts )); then
        apply_mode ac
    else
        apply_mode battery
    fi
}

apply_current_mode

if [[ "${1:-}" == "--once" ]]; then
    exit 0
fi

/usr/bin/pmset -g pslog | while IFS= read -r line; do
    case "$line" in
        *"Now drawing from '"*" Power'"*) apply_current_mode ;;
    esac
done
