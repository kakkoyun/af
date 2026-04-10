#!/usr/bin/env bash
# af bootstrap script for Arch Linux remotes.
# Installs core dependencies required by af workstreams.
# This script is embedded in the af binary via include_str!().
set -euo pipefail

echo "[af-bootstrap] Installing core dependencies (Arch Linux)..."

# Core tools
sudo pacman -Syu --noconfirm --needed git tmux

# Node.js (for Claude Code)
if ! command -v node &>/dev/null; then
    echo "[af-bootstrap] Installing Node.js..."
    sudo pacman -S --noconfirm --needed nodejs npm
fi

# GitHub CLI
if ! command -v gh &>/dev/null; then
    echo "[af-bootstrap] Installing GitHub CLI..."
    sudo pacman -S --noconfirm --needed github-cli
fi

# fzf
if ! command -v fzf &>/dev/null; then
    sudo pacman -S --noconfirm --needed fzf
fi

echo "[af-bootstrap] Done."
