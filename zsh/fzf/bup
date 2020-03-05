### BREW + FZF
# uninstall multiple packages at once, async
# mnemonic [B]rew [C]lean [P]lugin (e.g. uninstall)

local upd=$(brew leaves | eval "fzf ${FZF_DEFAULT_OPTS} -m --header='[brew:update]'")

if [[ $upd ]]; then
  for prog in $(echo $upd)
  do brew upgrade $prog
  done
fi

