#!/bin/bash

# Read the JSON input from stdin
input=$(cat)

# Pass the input to both commands and concatenate their outputs
echo "$input" | ccusage statusline
echo "$input" | ccstatusline
