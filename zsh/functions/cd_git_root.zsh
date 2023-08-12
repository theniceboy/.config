function gr() {
    local git_root
    git_root=$(git rev-parse --show-toplevel 2>/dev/null)
    if [[ $? -eq 0 && -d $git_root ]]; then
        cd "$git_root"
    else
        echo "Not in a git repository or could not find the git root."
    fi
}

