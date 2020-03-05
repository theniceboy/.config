tmux_preexec() {
    local tmux_event=${TMUX%%,*}-event/client-attached-pane
    if [[ -f $tmux_event-$TMUX_PANE ]]; then
        eval $(tmux showenv -s)
        command rm $tmux_event-$TMUX_PANE 2>/dev/null
    fi
}

if [[ -n $TMUX ]]; then
    local tmux_event=${TMUX%%,*}-event/client-attached-pane
    command rm $tmux_event-$TMUX_PANE 2>/dev/null

    autoload -U add-zsh-hook
    add-zsh-hook preexec tmux_preexec
fi
