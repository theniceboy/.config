setopt nopromptbang prompt{cr,percent,sp,subst}

COLOR_NORMAL=default
if [[ -n ${COLORFGBG:-} && ${COLORFGBG##*;} == <-> ]]; then
	_prompt_background=${COLORFGBG##*;}
	if (( _prompt_background == 7 || _prompt_background >= 10 )); then
		USER_LEVEL=${COLOR_USER_LIGHT:-blue}
	else
		USER_LEVEL=${COLOR_USER_DARK:-cyan}
	fi
elif [[ $(defaults read -g AppleInterfaceStyle 2>/dev/null) == Dark ]]; then
	USER_LEVEL=${COLOR_USER_DARK:-cyan}
else
	USER_LEVEL=${COLOR_USER_LIGHT:-blue}
fi
(( EUID )) || USER_LEVEL=${COLOR_ROOT:-red}
unset _prompt_background

zstyle ':zim:duration-info' threshold 0.5
zstyle ':zim:duration-info' format '%.4d s'

autoload -Uz add-zsh-hook
add-zsh-hook preexec duration-info-preexec
add-zsh-hook precmd duration-info-precmd

RPS1='${duration_info}%'
