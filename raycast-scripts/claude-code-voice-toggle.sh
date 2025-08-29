#!/bin/bash

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title Toggle Claude Voice
# @raycast.mode compact

# Optional parameters:
# @raycast.icon 🔊
# @raycast.description Toggle Claude Code voice on/off globally

voice_flag="$HOME/.claude/voice-enabled"

if [ -f "$voice_flag" ]; then
    # Voice is currently on, turn it off
    rm -f "$voice_flag"
    pkill -f 'say' 2>/dev/null
    echo "Claude Voice OFF"
else
    # Voice is currently off, turn it on
    touch "$voice_flag"
    echo "Claude Voice ON"
fi