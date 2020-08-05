# 我的配置文件...

[English Ver.](./README.md)

此份配置文件包含 `i3` 和 `alacritty` 的配置, 不管怎样, 我现在已在使用 [dwm](https://github.com/theniceboy/dwm) 和 [st](https://github.com/theniceboy/st)

顺便说一句, 我的脚本都在[此仓库中](https://github.com/theniceboy/scripts)

中文翻译: [KiteAB](https://github.com/KiteAB)

# 重要的东西:
## Ranger
使用 `pip install ueberzug` 和 `ranger-git`

## mutt
在 `~/.gnupg/gpg-agent.conf` 中:
```
default-cache-ttl 34560000
max-cache-ttl 34560000
```
如果它不能运行, 请尝试 [pam-gnupg](https://github.com/cruegge/pam-gnupg)
```
yay -S pam-gnupg-git
```
并在 `/etc/pam.d/system-local-login` 中添加以下行:
```
auth     optional  pam_gnupg.so
session  optional  pam_gnupg.so
```

## 输入法
安装: `fcitx` `fcitx-im` `fcitx-googlepinyin` `fcitx-configtool`

并编辑 `/etc/X11/xinit/xinitrc`:
```
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export XMODIFIERS="@im=fcitx"
```

#### Fcitx 用户需把第一输入法设置为键盘 - 布局

## 字体
#### 本地化
在 `locale.conf` 中:
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

#### 我的字体
我使用 `Source Code Pro` 字体和 `nerd-fonts-source-code-pro`

#### 关于 Noto
只需要安装 `noto-fonts` (并非 `-all`), 它已经非常臃肿, 检查 `/usr/share/fonts/noto`

#### Emoji
```
yay -S ttf-linux-libertine ttf-inconsolata ttf-joypixels ttf-twemoji-color noto-fonts-emoji ttf-liberation ttf-droid
```

#### 中文
```
yay -S wqy-bitmapfont wqy-microhei wqy-microhei-lite wqy-zenhei adobe-source-han-mono-cn-fonts adobe-source-han-sans-cn-fonts adobe-source-han-serif-cn-fonts
```

## GTK 主题
我使用 `adapta-gtk-theme` 与 `arc-icon-theme`

## 我安装的 Arch 软件包:
查看 [my-packages.txt](https://github.com/theniceboy/.config/blob/master/my-packages.txt)
