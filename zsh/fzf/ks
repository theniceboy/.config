### SERVER
# mnemonic: [K]ill [S]erver
# show output of "lsof -Pwni tcp", use [tab] to select one or multiple entries
# press [enter] to kill selected processes and go back to the process list.
# or press [escape] to go back to the process list. Press [escape] twice to exit completely.

local pid=$(lsof -Pwni tcp | sed 1d | eval "fzf ${FZF_DEFAULT_OPTS} -m --header='[kill:tcp]'" | awk '{print $2}')

if [ "x$pid" != "x" ]
then
  echo $pid | xargs kill -${1:-9}
  ks
fi

