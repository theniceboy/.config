#!/bin/bash

# Get current session cost (assuming current working directory as session)
session_data=$(ccusage session --json | jq -r '.sessions[] | select(.sessionId == "'"$(pwd | sed 's|/|-|g')"'") | .totalCost')
session_cost=${session_data:-0}

# Get today's cost
today_cost=$(ccusage daily --json | jq -r '.daily[0].totalCost // 0')

# Get current active block info
block_info=$(ccusage blocks --json | jq -r '
  .blocks[] | 
  select(.isActive == true) | 
  "\(.costUSD // 0)|\(.startTime)|\(.endTime)"
')

if [[ -n "$block_info" ]]; then
    block_cost=$(echo "$block_info" | cut -d'|' -f1)
    start_time=$(echo "$block_info" | cut -d'|' -f2)
    end_time=$(echo "$block_info" | cut -d'|' -f3)
    
    # Calculate time left in block (5 hours from start)
    start_epoch=$(date -j -f "%Y-%m-%dT%H:%M:%S.%fZ" "$start_time" +%s 2>/dev/null || date -d "$start_time" +%s 2>/dev/null || echo "0")
    current_epoch=$(date +%s)
    elapsed_seconds=$((current_epoch - start_epoch))
    remaining_seconds=$((18000 - elapsed_seconds)) # 5 hours = 18000 seconds
    
    if [[ $remaining_seconds -gt 0 ]]; then
        hours=$((remaining_seconds / 3600))
        minutes=$(((remaining_seconds % 3600) / 60))
        time_left="${hours}h ${minutes}m left"
    else
        time_left="expired"
    fi
    
    # Calculate hourly rate
    if [[ $elapsed_seconds -gt 0 ]]; then
        hourly_rate=$(echo "scale=2; $block_cost * 3600 / $elapsed_seconds" | bc -l 2>/dev/null || echo "0.00")
    else
        hourly_rate="0.00"
    fi
else
    block_cost="0.00"
    time_left="no active block"
    hourly_rate="0.00"
fi

# Format output
printf "💰 \$%.2f session / \$%.2f today / \$%.2f block (%s) | 🔥 \$%.2f/hr\n" \
    "$session_cost" "$today_cost" "$block_cost" "$time_left" "$hourly_rate"