# My config...

[中文版](./README_cn.md)

This config folder includes `i3` and `alacritty` config, however, I'm using [dwm](https://github.com/theniceboy/dwm) and [st](https://github.com/theniceboy/st) now.

BTW, my scripts are in [this repo](https://github.com/theniceboy/scripts).

# Brew packages
```
# Building
brew install automake gcc gdb jsdoc3 cmake gnu-getopt gnu-sed node go

# Utils
brew install git git-delta git-flow rainbarf bat ccat wget tree fzf the_silver_searcher ripgrep fd

# Apps
brew install tmux neovim jesseduffield/lazygit/lazygit yazi gh awscli tldr speedtest-cli ncdu neofetch onefetch bmon loc

# Yazi
brew install poppler ffmpeg sevenzip jq starship imagemagick
```

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

## Claude Code Voice Configuration

This config includes a directory-based voice system for Claude Code that uses macOS text-to-speech.

### Voice Commands
- `/voice-on` - Enable text-to-speech for current directory
- `/voice-off` - Disable text-to-speech for current directory

### Selecting and Downloading High Quality System Voices

For the best text-to-speech experience, download high-quality system voices:

1. **Open System Preferences** → **Accessibility** → **Spoken Content**
2. **Click "System Voice"** dropdown → **Customize...**
3. **Download premium voices** (these are much higher quality than default):
   - **English**: Alex (Enhanced), Samantha (Enhanced), Victoria (Enhanced)
   - **Other languages**: Download enhanced versions as needed
4. **Select your preferred voice** in the System Voice dropdown

**Note**: Enhanced voices are 100-200MB each but provide significantly better speech quality than compact voices.

### Voice Database
Voice settings are stored per-directory in `~/.claude/voice-db.json` and automatically created by the scripts.

## Arch Packages I Installed:
See [my-packages.txt](https://github.com/theniceboy/.config/blob/master/my-packages.txt)
