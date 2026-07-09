function zle_eval {
    echo -en "\e[2K\r"
    eval "$@"
    zle redisplay
}

function openlazygit {
    zle_eval lazygit
}

zle -N openlazygit; bindkey "^G" openlazygit

function openlazynpm {
    zle_eval lazynpm
}

zle -N openlazynpm; bindkey "^N" openlazynpm

autoload -Uz add-zsh-hook
_rebind_custom_keys() {
    bindkey '^p' fzf-find-widget
    bindkey '^n' openlazynpm
    add-zsh-hook -d precmd _rebind_custom_keys
}
add-zsh-hook precmd _rebind_custom_keys

