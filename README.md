# My Config

[中文版](./README_cn.md)

This config folder includes configurations for various development tools and applications.

## Deploy

### Quick Setup
Run the deployment script to install all tools and configure symlinks:
```bash
bin/upgrade-all
```

The script is idempotent - you can run it multiple times safely. It will:
- Install/update Homebrew packages
- Set up zsh configuration sourcing
- Create configuration symlinks (tmux, claude)
- Skip already installed packages

### Manual Homebrew Installation
If you need to install Homebrew manually:
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

### Iterm2 config
Title should be set to session name only. Do not allow other applications to change the title.

## Apps

### Claude Code Voice Configuration

This config includes a global voice system for Claude Code that uses macOS text-to-speech.

#### Voice Commands
- `/voice-on` - Enable text-to-speech globally
- `/voice-off` - Disable text-to-speech globally

#### Raycast Integration
For quick voice control, use the included Raycast scripts:
- **"Toggle Claude Voice"** - Toggle voice on/off from anywhere
- **"Stop Voice (TTS)"** - Immediately stop any playing speech

To add to Raycast: Add the `raycast-scripts/` directory to your Raycast script directories.

#### Selecting and Downloading High Quality System Voices

For the best text-to-speech experience, download high-quality system voices:

1. **Open System Preferences** → **Accessibility** → **Spoken Content**
2. **Click the info icon (ⓘ)** next to the "System Voice" dropdown
3. **Search for "Siri"** to find the highest quality voices
4. **Download Siri voices** - these are the premium, neural-powered voices
5. **Select your preferred Siri voice** in the System Voice dropdown

**Recommended**: Siri voices provide the most natural speech quality but require downloading additional voice data.

#### Voice Settings
Voice is controlled by a global flag file at `~/.claude/voice-enabled`. When this file exists, Claude Code will speak all responses.

### Other Applications
- **tmux**: Terminal multiplexer with custom configuration (set `TMUX_RAINBARF=0` before launching tmux to hide the rainbarf status segment)
- **neovim**: Modern text editor
- **yazi**: Terminal file manager
- **lazygit**: TUI for git operations

## Linux Related

<details>
<summary>Legacy Configuration (Click to expand)</summary>

My scripts are in [this repo](https://github.com/theniceboy/scripts).

This folder includes `i3` and `alacritty` config, however, I'm using [dwm](https://github.com/theniceboy/dwm) and [st](https://github.com/theniceboy/st) now.

### Ranger
Use `pip install ueberzug` and `ranger-git`

### Mutt Email Setup
In `~/.gnupg/gpg-agent.conf`:
```
default-cache-ttl 34560000
max-cache-ttl 34560000
```

If this doesn't work, try [pam-gnupg](https://github.com/cruegge/pam-gnupg):
```bash
yay -S pam-gnupg-git
```

And in `/etc/pam.d/system-local-login` add:
```
auth     optional  pam_gnupg.so
session  optional  pam_gnupg.so
```

### Input Methods
Install: `fcitx` `fcitx-im` `fcitx-googlepinyin` `fcitx-configtool`

And in `/etc/X11/xinit/xinitrc`:
```bash
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export XMODIFIERS="@im=fcitx"
```

**Note**: Fcitx users need to set the first input method to be Keyboard - layout

### Fonts

#### Locale Configuration
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

#### Font Recommendations
- **Main Font**: `Source Code Pro` and `nerd-fonts-source-code-pro`
- **Noto Fonts**: Install `noto-fonts` (not `-all` - it's bloated). Check `/usr/share/fonts/noto`

#### Emoji Fonts
```bash
yay -S ttf-linux-libertine ttf-inconsolata ttf-joypixels ttf-twemoji-color noto-fonts-emoji ttf-liberation ttf-droid
```

#### Chinese Fonts
```bash
yay -S wqy-bitmapfont wqy-microhei wqy-microhei-lite wqy-zenhei adobe-source-han-mono-cn-fonts adobe-source-han-sans-cn-fonts adobe-source-han-serif-cn-fonts
```

### GTK Theme
Using `adapta-gtk-theme` and `arc-icon-theme`.

### Arch Packages
See [my-packages.txt](https://github.com/theniceboy/.config/blob/master/my-packages.txt) for complete package list.

</details>
