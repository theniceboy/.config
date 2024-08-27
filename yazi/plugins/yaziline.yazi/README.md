# yaziline.yazi

Simple lualine-like status line for yazi.

Read more about features and configuration [here](#features).

![preview](https://github.com/llanosrocas/yaziline.yazi/blob/master/.github/images/preview.png)

## Requirements

- yazi version >= 0.3.0
- Font with symbol support. For example [Nerd Fonts](https://www.nerdfonts.com/).

## Installation

```sh
ya pack -a llanosrocas/yaziline
```

Or manually copy `init.lua` to the `~/.config/yazi/plugins/yaziline.yazi/init.lua`

## Usage

Add this to your `~/.config/yazi/init.lua`:

```lua
require("yaziline"):setup()
```

Optionally, configure line:

```lua
require("yaziline"):setup({
  separator_style = "angly" -- "angly" | "curvy" | "liney" | "empty"
  separator_open = "",
  separator_close = "",
  separator_open_thin = "",
  separator_close_thin = "",
  select_symbol = "",
  yank_symbol = "󰆐",
  filename_max_length = 24, -- trim when filename > 24
  filename_trim_length = 6 -- trim 6 chars from both ends
})
```

## Features

### Preconfigured separators

Choose your style:

- `angly`
  ![angly](https://github.com/llanosrocas/yaziline.yazi/blob/master/.github/images/angly.png)
- `curvy`
  ![curvy](https://github.com/llanosrocas/yaziline.yazi/blob/master/.github/images/curvy.png)
- `liney`
  ![liney](https://github.com/llanosrocas/yaziline.yazi/blob/master/.github/images/liney.png)
- `empty`
  ![empty](https://github.com/llanosrocas/yaziline.yazi/blob/master/.github/images/empty.png)

### Separator customization

You can provide your own symbols for separators combined with preconfigured separators. For example:

```lua
require("yaziline"):setup({
  -- Optinal config
  separator_style = "angly", -- preconfigured style
  separator_open = "", -- instead of 
  separator_close = "", -- instead of 
  separator_open_thin = "", -- change to anything
  separator_close_thin = "", -- change to anything
})
```

![empty](https://github.com/llanosrocas/yaziline.yazi/blob/master/.github/images/separator-combination.png)

_You can find more symbols [here](https://www.nerdfonts.com/cheat-sheet)_

### File actions icons

You can provide your own symbols for `select` and `yank`. For example:

```lua
require("yaziline"):setup({
  -- Optinal config
  select_symbol = "", -- "S" by default
  yank_symbol = "󰆐", -- "Y" by default
})
```

![empty](https://github.com/llanosrocas/yaziline.yazi/blob/master/.github/images/file-actions.png)

_You can find more symbols [here](https://www.nerdfonts.com/cheat-sheet)_

### Colors and font weight

You can change background and font weight in your `yazi/flavors/flavor.toml`.

```toml
mode_normal = { bg = "#98c379", bold = false }
```

For example, here is how my line looks like:

![preview-2](https://github.com/llanosrocas/yaziline.yazi/blob/master/.github/images/preview-2.png)

### Selected and Yanked Counter

Displays the number of selected ('S') and yanked ('Y') files on the left. If files are cut, the yank counter changes color, since its `yank --cut` under the hood.

### Trimmed Filename

Displays the trimmed filename on the left, which is useful for smaller screens or long filenames. By default, it's 24 characters with trimming to 12. Adjust in the `setup`.

```lua
require("yaziline"):setup({
  filename_max_length = 24, -- trim when filename > 24
  filename_trim_length = 6 -- trim 6 chars from both ends
})
```

### ISO Date for 'Modified'

On the right, you'll find the date and time the file was modified, formatted in an [ISO](https://en.wikipedia.org/wiki/ISO_8601)-like string for universal date representation. Adjust in the `Status:date` function.

## Credits

- [yazi source code](https://github.com/sxyazi/yazi)
- [yatline.yazi](https://github.com/imsi32/yatline.yazi/tree/main)
- [lualine.nvim](https://github.com/nvim-lualine/lualine.nvim)
