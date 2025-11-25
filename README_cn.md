# 我的配置

[English Ver.](./README.md)

此配置文件夹包含各种开发工具和应用程序的配置。

中文翻译: [KiteAB](https://github.com/KiteAB)

## 部署

### 快速设置
运行部署脚本来安装所有工具并配置符号链接：
```bash
bin/upgrade-all
```

脚本具有幂等性 - 你可以多次安全地运行它。它将：
- 安装/更新 Homebrew 包
- 设置 zsh 配置源
- 创建配置符号链接 (tmux, claude)
- 跳过已安装的包

### 手动安装 Homebrew
如果需要手动安装 Homebrew：
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

## 应用程序

### Claude Code 语音配置

此配置包含一个全局的 Claude Code 语音系统，使用 macOS 文本转语音功能。

#### 语音命令
- `/voice-on` - 全局启用文本转语音
- `/voice-off` - 全局禁用文本转语音

#### Raycast 集成
为了快速语音控制，使用包含的 Raycast 脚本：
- **"Toggle Claude Voice"** - 从任何地方切换语音开/关
- **"Stop Voice (TTS)"** - 立即停止任何正在播放的语音

添加到 Raycast：将 `raycast-scripts/` 目录添加到您的 Raycast 脚本目录。

#### 选择和下载高质量系统语音

为了获得最佳的文本转语音体验，请下载高质量的系统语音：

1. **打开系统偏好设置** → **辅助功能** → **朗读内容**
2. **点击"系统语音"下拉菜单旁边的信息图标 (ⓘ)**
3. **搜索"Siri"**以找到最高质量的语音
4. **下载 Siri 语音** - 这些是基于神经网络的高级语音
5. **在系统语音下拉菜单中选择你喜欢的 Siri 语音**

**推荐**: Siri 语音提供最自然的语音质量，但需要下载额外的语音数据。

#### 语音设置
语音由全局标志文件 `~/.claude/voice-enabled` 控制。当此文件存在时，Claude Code 将朗读所有响应。

### 其他应用程序
- **tmux**: 带有自定义配置的终端复用器
- **neovim**: 现代文本编辑器
- **yazi**: 终端文件管理器
- **lazygit**: Git 操作的 TUI

## Linux 相关

<details>
<summary>传统配置（点击展开）</summary>

我的脚本在[此仓库中](https://github.com/theniceboy/scripts)。

此文件夹包含 `i3` 和 `alacritty` 配置，不过我现在使用 [dwm](https://github.com/theniceboy/dwm) 和 [st](https://github.com/theniceboy/st)。

### Ranger
使用 `pip install ueberzug` 和 `ranger-git`

### Mutt 邮件设置
在 `~/.gnupg/gpg-agent.conf` 中：
```
default-cache-ttl 34560000
max-cache-ttl 34560000
```

如果这不起作用，请尝试 [pam-gnupg](https://github.com/cruegge/pam-gnupg)：
```bash
yay -S pam-gnupg-git
```

并在 `/etc/pam.d/system-local-login` 中添加：
```
auth     optional  pam_gnupg.so
session  optional  pam_gnupg.so
```

### 输入法
安装：`fcitx` `fcitx-im` `fcitx-googlepinyin` `fcitx-configtool`

并在 `/etc/X11/xinit/xinitrc` 中：
```bash
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export XMODIFIERS="@im=fcitx"
```

**注意**: Fcitx 用户需要将第一输入法设置为键盘 - 布局

### 字体

#### 本地化配置
在 `locale.conf` 中：
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

#### 字体推荐
- **主要字体**: `Source Code Pro` 和 `nerd-fonts-source-code-pro`
- **Noto 字体**: 安装 `noto-fonts`（不是 `-all` - 它很臃肿）。检查 `/usr/share/fonts/noto`

#### Emoji 字体
```bash
yay -S ttf-linux-libertine ttf-inconsolata ttf-joypixels ttf-twemoji-color noto-fonts-emoji ttf-liberation ttf-droid
```

#### 中文字体
```bash
yay -S wqy-bitmapfont wqy-microhei wqy-microhei-lite wqy-zenhei adobe-source-han-mono-cn-fonts adobe-source-han-sans-cn-fonts adobe-source-han-serif-cn-fonts
```

### GTK 主题
使用 `adapta-gtk-theme` 和 `arc-icon-theme`。

### Arch 软件包
查看 [my-packages.txt](https://github.com/theniceboy/.config/blob/master/my-packages.txt) 获取完整软件包列表。

</details>
