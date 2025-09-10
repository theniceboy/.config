#!/bin/bash

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title Toggle Claude Ring Mode
# @raycast.mode compact

# Optional parameters:
# @raycast.icon 🔔
# @raycast.description Toggle Claude Code ring mode on/off (plays "Claude Code Done" sound)

ring_flag="$HOME/.claude/ring-enabled"
voice_flag="$HOME/.claude/voice-enabled"

if [ -f "$ring_flag" ]; then
    # Ring mode is currently on, turn it off
    rm -f "$ring_flag"
    pkill -f 'say' 2>/dev/null
    echo "Claude Ring Mode OFF"
else
    # Ring mode is currently off, turn it on
    # First disable voice mode if it's enabled
    if [ -f "$voice_flag" ]; then
        rm -f "$voice_flag"
    fi
    touch "$ring_flag"
    echo "Claude Ring Mode ON (Voice Mode disabled)"
fi