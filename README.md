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

## fonts
- Just install `noto-fonts`. It's already bloated. Check `/usr/share/fonts/noto`
#### Emoji
```
ttf-linux-libertine # probably not actually needed, only aesthetic
ttf-inconsolata # the monospace font
ttf-emojione
ttf-symbola
ttf-joypixels
ttf-twemoji-color
```
#### Chinese
```
wqy-bitmapfont 1.0.0RC1-3
wqy-microhei 0.2.0_beta-9
wqy-microhei-lite 0.2.0_beta-9
wqy-zenhei 0.9.45-7
```

## gtk-theme
I use `adapta-gtk-theme` and `arc-icon-theme`.UUU

## Arch Packages I Installed:
See [my-packages.txt](https://github.com/theniceboy/.config/blob/master/my-packages.txt)
