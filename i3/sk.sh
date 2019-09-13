result=$(ps ax|grep -v grep|grep screenkey)
if [ "$result" == "" ]; then
  eval "screenkey --bg-color white --font-color black &"
else
  eval "killall screenkey"
fi
