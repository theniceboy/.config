# starship.yazi

Starship prompt plugin for [Yazi](https://github.com/sxyazi/yazi)

<https://github.com/Rolv-Apneseth/starship.yazi/assets/69486699/f7314687-5cb1-4d66-8d9d-cca960ba6716>

## Requirements

- [Yazi](https://github.com/sxyazi/yazi) - latest main branch
- [starship](https://github.com/starship/starship)

### Package manager

```bash
ya pack -a Rolv-Apneseth/starship
```

### Manual

#### Linux / MacOS

```sh
git clone https://github.com/Rolv-Apneseth/starship.yazi.git ~/.config/yazi/plugins/starship.yazi
```

#### Windows

```sh
git clone https://github.com/Rolv-Apneseth/starship.yazi.git %AppData%\yazi\config\plugins\starship.yazi
```

## Usage

Add this to `~/.config/yazi/init.lua`:

```lua
require("starship"):setup()
```

If you wish to define a custom config file for `starship` to use, you can pass in a path
to the setup function like this:

```lua
starship:setup({ config_file = "/home/rolv/.config/starship_secondary.toml" })
```

Make sure you have [starship](https://github.com/starship/starship) installed and in your `PATH`.

## Extra

If you use a `starship` theme with a background colour, it might look a bit to cramped on just the one line `Yazi` gives the header by default. To fix this, you can add this to your `init.lua`:

<details>
<summary>Click to expand</summary>

```lua
local old_build = Tab.build
Tab.build = function(self, ...)
    local bar = function(c, x, y)
        if x <= 0 or x == self._area.w - 1 then
            return ui.Bar(ui.Rect.default, ui.Bar.TOP)
        end

        return ui.Bar(
            ui.Rect({
                x = x,
                y = math.max(0, y),
                w = ya.clamp(0, self._area.w - x, 1),
                h = math.min(1, self._area.h),
            }),
            ui.Bar.TOP
        ):symbol(c)
    end

    local c = self._chunks
    self._chunks = {
        c[1]:padding(ui.Padding.y(1)),
        c[2]:padding(ui.Padding(c[1].w > 0 and 0 or 1, c[3].w > 0 and 0 or 1, 1, 1)),
        c[3]:padding(ui.Padding.y(1)),
    }

    local style = THEME.manager.border_style
    self._base = ya.list_merge(self._base or {}, {
        -- Enable for full border
        --[[ ui.Border(self._area, ui.Border.ALL):type(ui.Border.ROUNDED):style(style), ]]
        ui.Bar(self._chunks[1], ui.Bar.RIGHT):style(style),
        ui.Bar(self._chunks[3], ui.Bar.LEFT):style(style),

        bar("┬", c[1].right - 1, c[1].y),
        bar("┴", c[1].right - 1, c[1].bottom - 1),
        bar("┬", c[2].right, c[2].y),
        bar("┴", c[2].right, c[1].bottom - 1),
    })

    old_build(self, ...)
end
```

</details>

> [!NOTE]
> This works by overriding your `Tab.build` function so make sure this is the only place you're doing that in your config. For example, this would be incompatible with the [full-border plugin](https://github.com/yazi-rs/plugins/tree/main/full-border.yazi)

## Thanks

- [sxyazi](https://github.com/sxyazi) for providing the code for this plugin and the demo video [in this comment](https://github.com/sxyazi/yazi/issues/767#issuecomment-1977082834)
