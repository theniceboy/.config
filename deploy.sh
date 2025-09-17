#!/bin/bash

set -euo pipefail

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

# Run upgrade-all to install/update packages
echo "📦 Running upgrade-all to install/update packages..."
if command -v upgrade-all &> /dev/null; then
    upgrade-all
elif [ -x "$HOME/.config/bin/upgrade-all" ]; then
    python3 "$HOME/.config/bin/upgrade-all"
else
    echo "❌ upgrade-all script not found"
    echo "   Expected at: $HOME/.config/bin/upgrade-all"
    exit 1
fi

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

    if [ -L "$link_name" ]; then
        local current
        current=$(readlink "$link_name")
        if [ "$current" = "$target" ]; then
            echo "✅ $display_name symlink already exists and is correct"
            return 0
        fi
        echo "⚠️  $link_name points to $current; updating to $target"
        rm "$link_name"
    elif [ -e "$link_name" ]; then
        local backup="${link_name}.backup"
        if [ -e "$backup" ]; then
            backup="${backup}.$(date +%Y%m%d%H%M%S)"
        fi
        echo "⚠️  Backing up existing $link_name to $backup"
        mv "$link_name" "$backup"
    fi

    ln -s "$target" "$link_name"
    echo "✅ Symlink ensured: $link_name -> $target"
}

bash ~/.config/agent-tracker/scripts/install_brew_service.sh
# Create configuration symlinks
create_symlink "$HOME/.config/.tmux.conf" "$HOME/.tmux.conf" "Tmux"
create_symlink "$HOME/.config/claude" "$HOME/.claude" "Claude"
create_symlink "$HOME/.config/codex" "$HOME/.codex" "Codex"

echo "🎉 Deployment complete!"
