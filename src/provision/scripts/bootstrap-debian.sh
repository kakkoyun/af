#!/usr/bin/env bash
# af bootstrap script for Debian/Ubuntu remotes.
# Installs core dependencies required by af workstreams.
# This script is embedded in the af binary via include_str!().
set -euo pipefail

echo "[af-bootstrap] Installing core dependencies (Debian/Ubuntu)..."

export DEBIAN_FRONTEND=noninteractive

# Core tools
sudo apt-get update -qq
sudo apt-get install -y -qq git tmux curl

# Node.js (for Claude Code)
if ! command -v node &>/dev/null; then
    echo "[af-bootstrap] Installing Node.js via NodeSource..."
    curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
    sudo apt-get install -y -qq nodejs
fi

# GitHub CLI
if ! command -v gh &>/dev/null; then
    echo "[af-bootstrap] Installing GitHub CLI..."
    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
    sudo apt-get update -qq
    sudo apt-get install -y -qq gh
fi

# fzf
if ! command -v fzf &>/dev/null; then
    sudo apt-get install -y -qq fzf 2>/dev/null || true
fi

echo "[af-bootstrap] Done."
