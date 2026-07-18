#!/usr/bin/env bash
set -euo pipefail

state_root="${XDG_STATE_HOME:-$HOME/.local/state}"
rm -f "$state_root/op/.lastmsg-prune-stamp"
