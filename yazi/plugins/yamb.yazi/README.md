# Yet another bookmarks

A [Yazi](https://github.com/sxyazi/yazi) plugin for bookmark management, supporting the following features

- Persistent bookmarks. No bookmarks are lost after you close yazi.
- Quickly jump, delete, and rename a bookmark by keymap.
- Support fuzzy search through [fzf](https://github.com/junegunn/fzf).
- Configure your bookmarks using Lua language.

## Installation

> [!NOTE]
> Yazi >= 0.25.

```sh
# Linux/macOS
git clone https://github.com/h-hg/yamb.yazi.git ~/.config/yazi/plugins/yamb.yazi

# Windows
git clone https://github.com/h-hg/yamb.yazi.git $env:APPDATA\yazi\config\plugins\yamb.yazi

# if you are using Yazi version >= 3.0
ya pack -a h-hg/yamb
```

## Usage

Add this to your `init.lua`

```lua
-- You can configure your bookmarks by lua language
local bookmarks = {}

local path_sep = package.config:sub(1, 1)
local home_path = ya.target_family() == "windows" and os.getenv("USERPROFILE") or os.getenv("HOME")
if ya.target_family() == "windows" then
  table.insert(bookmarks, {
    tag = "Scoop Local",
    
    path = (os.getenv("SCOOP") or home_path .. "\\scoop") .. "\\",
    key = "p"
  })
  table.insert(bookmarks, {
    tag = "Scoop Global",
    path = (os.getenv("SCOOP_GLOBAL") or "C:\\ProgramData\\scoop") .. "\\",
    key = "P"
  })
end
table.insert(bookmarks, {
  tag = "Desktop",
  path = home_path .. path_sep .. "Desktop" .. path_sep,
  key = "d"
})

require("yamb"):setup {
  -- Optional, the path ending with path seperator represents folder.
  bookmarks = bookmarks,
  -- Optional, recieve notification everytime you jump.
  jump_notify = true,
  -- Optional, the cli of fzf.
  cli = "fzf",
  -- Optional, a string used for randomly generating keys, where the preceding characters have higher priority.
  keys = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
  -- Optional, the path of bookmarks
  path = (ya.target_family() == "windows" and os.getenv("APPDATA") .. "\\yazi\\config\\bookmark") or
        (os.getenv("HOME") .. "/.config/yazi/bookmark"),
}
```

Add this to your `keymap.toml`:

```toml
[[manager.prepend_keymap]]
on = [ "u", "a" ]
run = "plugin yamb save"
desc = "Add bookmark"

[[manager.prepend_keymap]]
on = [ "u", "g" ]
run = "plugin yamb jump_by_key"
desc = "Jump bookmark by key"

[[manager.prepend_keymap]]
on = [ "u", "G" ]
run = "plugin yamb jump_by_fzf"
desc = "Jump bookmark by fzf"

[[manager.prepend_keymap]]
on = [ "u", "d" ]
run = "plugin yamb delete_by_key"
desc = "Delete bookmark by key"

[[manager.prepend_keymap]]
on = [ "u", "D" ]
run = "plugin yamb delete_by_fzf"
desc = "Delete bookmark by fzf"

[[manager.prepend_keymap]]
on = [ "u", "A" ]
run = "plugin yamb delete_all"
desc = "Delete all bookmarks"

[[manager.prepend_keymap]]
on = [ "u", "r" ]
run = "plugin yamb rename_by_key"
desc = "Rename bookmark by key"

[[manager.prepend_keymap]]
on = [ "u", "R" ]
run = "plugin yamb rename_by_fzf"
desc = "Rename bookmark by fzf"
```
