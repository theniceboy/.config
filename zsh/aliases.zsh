alias av='source venv/bin/activate'
alias c='clear'
alias cdiff='colordiff'
alias cs='calcurse'
alias dv='deactivate'
alias gc='git config credential.helper store'
alias gg='git clone'
alias ipy='ipython'
alias l='ls -la'
alias lg='lazygit'
alias ms='mailsync'
alias mt='neomutt'
alias r='echo $RANGER_LEVEL'
alias pu='python3 -m pudb'
alias ra='yazi'
# ra() {
	#if [ -z "$RANGER_LEVEL" ]
	#then
		#ranger
	#else
		#exit
	#fi
#}
alias s='neofetch'
alias g='onefetch'
alias sra='sudo -E yazi'
# alias sudo='sudo -E'
alias vim='nvim'
alias gs='git config credential.helper store'
alias ac='sudo tlp ac'
alias gy='git-yolo'
alias nb='newsboat -r'
alias nt="sh -c 'cd $(pwd); st' > /dev/null 2>&1 &"
alias ta='tmux a'
alias t='tmux'
alias lo='lsof -p $(fps) +w'
alias py="python"
alias cl='claude --dangerously-skip-permissions --append-system-prompt "$(cat ~/.config/claude/system-prompt.txt)"'
alias co="codex --sandbox danger-full-access -m gpt-5-codex -c model_reasoning_effort=\"high\""
