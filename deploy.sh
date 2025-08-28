#!/bin/bash

set -e

echo "🚀 Starting deployment script..."

# Check if brew is installed or try to find it
if ! command -v brew &> /dev/null; then
    echo "⚠️  Homebrew not in PATH, checking common locations..."
    
    # Try common Homebrew paths for macOS
    if [[ "$OSTYPE" == "darwin"* ]]; then
        for brew_path in "/opt/homebrew/bin/brew" "/usr/local/bin/brew"; do
            if [[ -x "$brew_path" ]]; then
                echo "🔧 Found Homebrew at $brew_path, setting up environment..."
                eval "$($brew_path shellenv)"
                break
            fi
        done
    fi
    
    # Check again after PATH update
    if ! command -v brew &> /dev/null; then
        echo "❌ Homebrew not found"
        echo "📦 Please install Homebrew first by running:"
        echo "    /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
        echo "After installation, make sure to run the commands it suggests to add brew to your PATH."
        echo "Then run this script again."
        exit 1
    fi
fi

echo "✅ Homebrew found"

# Install brew packages
echo "📦 Checking brew packages..."

# Get list of installed packages
installed_packages=$(brew list --formula -1)

# Function to install package if not already installed
install_if_missing() {
    local package="$1"
    local display_name="${2:-$package}"
    
    # Handle tap packages (contains /)
    if [[ "$package" == *"/"* ]]; then
        local package_name="${package##*/}"  # Extract package name after last /
        if ! echo "$installed_packages" | grep -q "^${package_name}$"; then
            echo "  📦 Installing $display_name..."
            brew install "$package"
        fi
    else
        if ! echo "$installed_packages" | grep -q "^${package}$"; then
            echo "  📦 Installing $display_name..."
            brew install "$package"
        fi
    fi
}

# System utilities
install_if_missing "htop" # Interactive process viewer
install_if_missing "dust" # Disk usage analyzer
install_if_missing "ncdu" # Disk usage analyzer with ncurses
install_if_missing "fswatch" # File system monitoring
install_if_missing "pipx" # Install Python apps in isolation

# macOS GNU utilities
if [[ "$OSTYPE" == "darwin"* ]]; then
    install_if_missing "coreutils" # GNU core utilities (ls, cp, mv, etc.)
    install_if_missing "gnu-tar" # GNU tar archiver
    install_if_missing "gnu-getopt" # GNU command line option parsing
    install_if_missing "gnu-sed" # GNU stream editor
fi

# Development tools
install_if_missing "node" # Node.js JavaScript runtime
install_if_missing "go" # Go programming language
install_if_missing "gcc" # GNU Compiler Collection
install_if_missing "gdb" # GNU debugger
install_if_missing "automake" # Build automation tool
install_if_missing "cmake" # Cross-platform build system
install_if_missing "jsdoc3" # JavaScript documentation generator

# Text processing and search
install_if_missing "ripgrep" # Fast text search (rg)
install_if_missing "the_silver_searcher" # Fast text search (ag)
install_if_missing "fd" # Fast find alternative
install_if_missing "fzf" # Fuzzy finder
install_if_missing "bat" # Better cat with syntax highlighting
install_if_missing "ccat" # Colorized cat
install_if_missing "tree" # Display directory trees
install_if_missing "jq" # JSON processor

# File management
install_if_missing "yazi" # Terminal file manager
install_if_missing "sevenzip" # File archiver

# Git and version control
install_if_missing "git" # Version control system
install_if_missing "git-delta" # Better git diff viewer
install_if_missing "git-flow" # Git branching model extensions
install_if_missing "jesseduffield/lazygit/lazygit" "lazygit" # TUI for git
install_if_missing "gh" # GitHub CLI

# Media and graphics
install_if_missing "ffmpeg" # Media processing toolkit
install_if_missing "imagemagick" # Image manipulation toolkit
install_if_missing "poppler" # PDF rendering library
install_if_missing "yt-dlp" # YouTube downloader

# Network and web
install_if_missing "wget" # Web file downloader
install_if_missing "speedtest-cli" # Internet speed test

# Terminal and shell
install_if_missing "tmux" # Terminal multiplexer
install_if_missing "neovim" # Modern Vim text editor
install_if_missing "starship" # Cross-shell prompt
install_if_missing "rainbarf" # CPU/RAM/battery stats for tmux
install_if_missing "neofetch" # System information display
install_if_missing "onefetch" # Git repository information
install_if_missing "tldr" # Simplified man pages
install_if_missing "bmon" # Bandwidth monitor
install_if_missing "loc" # Lines of code counter

# Applications
install_if_missing "awscli" # AWS command line interface
install_if_missing "azure-cli" # Azure command line interface
install_if_missing "terraform" # Infrastructure as code

echo "✅ All packages processed"

# Ensure zsh configuration is sourced
echo "🔗 Setting up zsh configuration..."
if [ ! -f "$HOME/.zshrc" ]; then
    echo "source ~/.config/zsh/zshrc" > "$HOME/.zshrc"
    echo "✅ Created ~/.zshrc with config source"
elif ! grep -q "source ~/.config/zsh/zshrc" "$HOME/.zshrc"; then
    echo "source ~/.config/zsh/zshrc" >> "$HOME/.zshrc"
    echo "✅ Added config source to ~/.zshrc"
else
    echo "✅ Zsh config source already exists in ~/.zshrc"
fi

# Function to create symlinks
create_symlink() {
    local target="$1"
    local link_name="$2"
    local display_name="$3"
    
    echo "🔗 Setting up $display_name symlink..."
    
    if [ ! -L "$link_name" ]; then
        if [ -e "$link_name" ]; then
            echo "⚠️  Backing up existing $link_name to ${link_name}.backup"
            mv "$link_name" "${link_name}.backup"
        fi
        ln -s "$target" "$link_name"
        echo "✅ Symlink created: $link_name -> $target"
    elif [ "$(readlink "$link_name")" = "$target" ]; then
        echo "✅ $display_name symlink already exists and is correct"
    else
        echo "⚠️  $link_name exists but points to $(readlink "$link_name")"
        echo "   Expected: $target"
    fi
}

# Create configuration symlinks
create_symlink "$HOME/.config/.tmux.conf" "$HOME/.tmux.conf" "Tmux"
create_symlink "$HOME/.config/claude" "$HOME/.claude" "Claude"

echo "🎉 Deployment complete!"
