<h1 align="center">🗜️ compress.yazi</h1>
<p align="center">
  <b>A blazing fast, flexible archive plugin for <a href="https://github.com/sxyazi/yazi">Yazi</a></b><br>
  <i>Effortlessly compress your files and folders with style!</i>
</p>

---

## 📖 Table of Contents

- [Features](#-features)
- [Supported File Types](#-supported-file-types)
- [Installation](#%EF%B8%8F-installation)
- [Keymap Example](#-keymap-example)
- [Usage](#%EF%B8%8F-usage)
- [Flags](#%EF%B8%8F-flags)
- [Tips](#-tips)
- [Credits](#-credits)

---

## 🚀 Features

- 🗂️ **Multi-format support:** zip, 7z, rar, tar, tar.gz, tar.xz, tar.bz2, tar.zst, tar.lz4, tar.lha
- 🌍 **Cross-platform:** Works on Unix & Windows
- 🔒 **Password protection:** Secure your archives (zip/7z/rar)
- 🛡️ **Header encryption:** Hide file lists (7z/rar)
- ⚡ **Compression level:** Choose your balance of speed vs. size
- 🛑 **Overwrite safety:** Never lose files by accident
- 🎯 **Seamless Yazi integration:** Fast, native-like UX

---

## 📦 Supported File Types

| Extension     | Default Command   | 7z Command     | Bsdtar Command (Win10+ & Unix) |
| ------------- | ----------------- | -------------- | ------------------------------ |
| `.zip`        | `zip -r`          | `7z a -tzip`   | `tar -caf`                     |
| `.7z`         | `7z a`            | `7z a`         |                                |
| `.rar`        | `rar a`           |                |                                |
| `.tar`        | `tar rpf`         |                | `tar rpf`                      |
| `.tar.gz`     | `tar rpf + gzip`  | `7z a -tgzip`  | `tar -czf`                     |
| `.tar.xz`     | `tar rpf + xz`    | `7z a -txz`    | `tar -cJf`                     |
| `.tar.bz2`    | `tar rpf + bzip2` | `7z a -tbzip2` | `tar -cjf`                     |
| `.tar.zst`    | `tar rpf + zstd`  |                | `tar --zstd -cf`               |
| `.tar.lz4`    | `tar rpf + lz4`   |                |                                |
| `.tar.lha`    | `tar rpf + lha`   |                |                                |

---

## ⚡️ Installation

```bash
# Unix
git clone https://github.com/KKV9/compress.yazi.git ~/.config/yazi/plugins/compress.yazi

# Windows (CMD, not PowerShell!)
git clone https://github.com/KKV9/compress.yazi.git %AppData%\yazi\config\plugins\compress.yazi

# Or with yazi plugin manager
ya pkg add KKV9/compress
```

---

### 🔧 Extras (Windows)

To enable additional compression formats and features on Windows, follow these steps:

1. **Install [7-Zip](https://www.7-zip.org/):**  
   Add `C:\Program Files\7-Zip` to your `PATH`.  
   This enables support for `.7z` archives and password-protected `.zip` files.

2. **Alternative: Install [Nanazip](https://github.com/M2Team/NanaZip):**  
   A modern alternative to 7-Zip with similar functionality and extra features.

3. **Install [WinRAR](https://www.win-rar.com/download.html):**  
   Add `C:\Program Files\WinRAR` to your `PATH`.  
   This enables support for `.rar` archives.

4. **Install Additional Tools:**  
   To use formats like `lha`, `lz4`, `gzip`, etc., install their respective tools and ensure they are added to your `PATH`.

---

## 🎹 Keymap Example

Add this to your `keymap.toml`:


```toml
[[mgr.prepend_keymap]]
on   = [ "c", "a", "a" ]
run  = "plugin compress"
desc = "Archive selected files"

[[mgr.prepend_keymap]]
on   = [ "c", "a", "p" ]
run  = "plugin compress -p"
desc = "Archive selected files (password)"

[[mgr.prepend_keymap]]
on   = [ "c", "a", "h" ]
run  = "plugin compress -ph"
desc = "Archive selected files (password+header)"

[[mgr.prepend_keymap]]
on   = [ "c", "a", "l" ]
run  = "plugin compress -l"
desc = "Archive selected files (compression level)"

[[mgr.prepend_keymap]]
on   = [ "c", "a", "u" ]
run  = "plugin compress -phl"
desc = "Archive selected files (password+header+level)"
```

---

## 🛠️ Usage

1. **Select files/folders** in Yazi.
2. Press <kbd>c</kbd> <kbd>a</kbd> to open the archive dialog.
3. Choose:
   - <kbd>a</kbd> for a standard archive
   - <kbd>p</kbd> for password protection (zip/7z/rar)
   - <kbd>h</kbd> to encrypt header (7z/rar)
   - <kbd>l</kbd> to set compression level (all compression algorithims)
   - <kbd>u</kbd> for all options together
4. **Type a name** for your archive (or leave blank for suggested name).
5. **Enter password** and/or **compression level** if prompted.
6. **Overwrite protect** if a file already exists, the new file will be given a suffix _#.
7. Enjoy your shiny new archive!

---

## 🏳️‍🌈 Flags

- Combine flags for more power!
- when separating flags with spaces, make sure to single quote them (eg., `'-ph rar'`)
- `-p` Password protect (zip/7z/rar)
- `-h` Encrypt header (7z/rar)
- `-l` Set compression level (all compression algorithims)
- `<extention>` Specify a default extention (eg., `7z`, `tar.gz`)

#### Combining multiple flags:
```toml
[[mgr.prepend_keymap]]
on   = [ "c", "a", "7" ]
run  = "plugin compress '-ph 7z'"
desc = "Archive selected files to 7z (password+header)"
[[mgr.prepend_keymap]]
on   = [ "c", "a", "r" ]
run  = "plugin compress '-p -l rar'"
desc = "Archive selected files to rar (password+level)"
```

---

## 💡 Tips

- The file extension **must** match a supported type.
- The required compression tool **must** be installed and in your `PATH` (7zip/rar etc.).
- If no extention is provided, the default extention (zip) will be appended automatically.

---

## 📣 Credits

Made with ❤️ for [Yazi](https://github.com/sxyazi/yazi) by [KKV9](https://github.com/KKV9).
Contributions are welcome! Feel free to submit a pull request.

---
