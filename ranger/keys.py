from ranger.api.keys import *

# keymanager.unmap("browser", "n")

map = vim_aliases = KeyMapWithDirections()
map('u', fm.move(up=1))
map('e', fm.move(down=1))

keymanager.merge_all(map)     # merge the new map into all existing ones.

