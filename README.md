# My config...
This config folder includes `i3` and `alacritty` config, however, I'm using [dwm](https://github.com/theniceboy/dwm) and [st](https://github.com/theniceboy/st) now.

# Important stuff:
## Ranger
use `ueberzug` and `ranger-git`

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
ttf-linux-libertine # probably not actually needed, only aesthetic
ttf-inconsolata # the monospace font
ttf-emojione
ttf-symbola
ttf-joypixels
ttf-twemoji-color
noto-fonts-emoji
```

#### Chinese
```
wqy-bitmapfont 1.0.0RC1-3
wqy-microhei 0.2.0_beta-9
wqy-microhei-lite 0.2.0_beta-9
wqy-zenhei 0.9.45-7
adobe-source-han-mono
adobe-source-han-sans
adobe-source-han-serif
```

## gtk-theme
I use `adapta-gtk-theme` and `arc-icon-theme`.

## Arch Packages I Installed:
See [my-packages.txt](https://github.com/theniceboy/.config/blob/master/my-packages.txt)
