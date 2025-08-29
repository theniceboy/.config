#!/bin/bash

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title Stop Voice (TTS)
# @raycast.mode compact

# Optional parameters:
# @raycast.icon 🔇
# @raycast.description Stop all text-to-speech playback

# Kill all running say processes
pkill -f 'say' 2>/dev/null

echo "All TTS stopped"