#!/bin/bash
current_dir=$(pwd)
voice_db="$HOME/.claude/voice-db.json"

# Create voice database if it doesn't exist
if [ ! -f "$voice_db" ]; then
    echo "{}" > "$voice_db"
fi

jq --arg dir "$current_dir" '.[$dir] = false' "$voice_db" > /tmp/voice-db-temp.json && mv /tmp/voice-db-temp.json "$voice_db"
echo "Voice disabled for $current_dir"