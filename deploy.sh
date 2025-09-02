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
