export GOPATH=$HOME/go
export PATH=$PATH:$HOME/.gem/ruby/2.6.0
export PATH=$PATH:/usr/local/bin
export PATH=$PATH:$HOME/go/bin
export PATH=$PATH:/home/linuxbrew/.linuxbrew/bin
export PATH=$PATH:$HOME/.local/bin
export PATH=$PATH:$HOME/.config/bin
export PATH=$PATH:/snap/bin
#export TERM=xterm-256color
export PATH=$PATH:/home/david/prog/flutter/bin
export PATH=$PATH:/Users/david/prog/flutter/bin
export TERM=xterm-256color
export TERM_ITALICS=true
export RANGER_LOAD_DEFAULT_RC="false"
#export TERM=screen-256color
export EDITOR=nvim
export ZSH_AUTOSUGGEST_USE_ASYNC=1
export ZSH_AUTOSUGGEST_MANUAL_REBIND=1

# fzf
export FZF_DEFAULT_OPTS='--bind ctrl-e:down,ctrl-u:up --preview "[[ $(file --mime {}) =~ binary ]] && echo {} is a binary file || (ccat --color=always {} || highlight -O ansi -l {} || cat {}) 2> /dev/null | head -500"'
export FZF_DEFAULT_COMMAND='ag --hidden --ignore .git -g ""'
export FZF_COMPLETION_TRIGGER='\'
export FZF_TMUX=1
export FZF_TMUX_HEIGHT='80%'
export fzf_preview_cmd='[[ $(file --mime {}) =~ binary ]] && echo {} is a binary file || (ccat --color=always {} || highlight -O ansi -l {} || cat {}) 2> /dev/null | head -500'
