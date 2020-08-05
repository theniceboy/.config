# My config...

[中文版](./README_cn.md)

This config folder includes `i3` and `alacritty` config, however, I'm using [dwm](https://github.com/theniceboy/dwm) and [st](https://github.com/theniceboy/st) now.

BTW, my scripts are in [this repo](https://github.com/theniceboy/scripts).

# Important stuff:
## Ranger
use `pip install ueberzug` and `ranger-git`

## mutt
In `~/.gnupg/gpg-agent.conf`:
```
default-cache-ttl 34560000
max-cache-ttl 34560000
```
If this doesn't work, try [pam-gnupg](https://github.com/cruegge/pam-gnupg)
```
yay -S pam-gnupg-git
```
and in `/etc/pam.d/system-local-login` add:
```
auth     optional  pam_gnupg.so
session  optional  pam_gnupg.so
```

## Input Methods
Install: `fcitx` `fcitx-im` `fcitx-googlepinyin` `fcitx-configtool`

And in `/etc/X11/xinit/xinitrc`:
```
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export XMODIFIERS="@im=fcitx"
```

#### Fcitx users need to set the first input method to be Keyboard - layout

## fonts
#### locale
In `locale.conf`:
```
LANG=en_US.UTF-8
LC_ADDRESS=en_US.UTF-8
LC_IDENTIFICATION=en_US.UTF-8
LC_MEASUREMENT=en_US.UTF-8
LC_MONETARY=en_US.UTF-8
LC_NAME=en_US.UTF-8
LC_NUMERIC=en_US.UTF-8
LC_PAPER=en_US.UTF-8
LC_TELEPHONE=en_US.UTF-8
LC_TIME=en_US.UTF-8
```

#### My Font
I use the `Source Code Pro` font and `nerd-fonts-source-code-pro`.

#### About Noto
Just install `noto-fonts` (not `-all`). It's already bloated. Check `/usr/share/fonts/noto`

#### Emoji
```
yay -S ttf-linux-libertine ttf-inconsolata ttf-joypixels ttf-twemoji-color noto-fonts-emoji ttf-liberation ttf-droid
```

#### Chinese
```
yay -S wqy-bitmapfont wqy-microhei wqy-microhei-lite wqy-zenhei adobe-source-han-mono-cn-fonts adobe-source-han-sans-cn-fonts adobe-source-han-serif-cn-fonts
```

## gtk-theme
I use `adapta-gtk-theme` and `arc-icon-theme`.

## Arch Packages I Installed:
See [my-packages.txt](https://github.com/theniceboy/.config/blob/master/my-packages.txt)
