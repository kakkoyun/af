//! Platform detection and dependency management (ADR-009, ADR-010).
//!
//! Detects the current platform (macOS, Arch Linux, Debian/Ubuntu) and provides
//! a package manager abstraction for installing dependencies.
//!
//! # Platform detection strategy
//!
//! - **macOS**: Detected via `std::env::consts::OS == "macos"`.
//! - **Arch Linux**: Detected by reading `/etc/os-release` and matching the `ID`
//!   field against known Arch-based distributions (arch, endeavouros, garuda,
//!   cachyos, artix).
//! - **Debian**: Fallback for all other Linux distributions.
//!
//! # Examples
//!
//! ```no_run
//! use af::platform::Platform;
//!
//! let platform = Platform::detect().expect("unsupported OS");
//! println!("Running on {} with {:?}", platform.display_name(), platform.package_manager());
//! ```

pub mod deps;

use std::fmt;

/// Errors that can occur during platform detection.
#[derive(Debug, thiserror::Error)]
pub enum PlatformError {
    /// The operating system is not supported by `af`.
    #[error("unsupported operating system: {os}")]
    UnsupportedOs {
        /// The name of the unsupported operating system.
        os: String,
    },

    /// Failed to read `/etc/os-release` on Linux.
    #[error("failed to read /etc/os-release: {source}")]
    OsReleaseRead {
        /// The underlying I/O error.
        #[source]
        source: std::io::Error,
    },
}

/// Supported operating system platforms.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Platform {
    /// macOS (Homebrew).
    MacOS,
    /// Arch Linux and derivatives (`EndeavourOS`, `Garuda`, `CachyOS`, `Artix`).
    Arch,
    /// Debian, Ubuntu, and all other Linux distributions (fallback).
    Debian,
}

/// Supported package managers.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PackageManager {
    /// Homebrew (macOS).
    Brew,
    /// Pacman (Arch Linux).
    Pacman,
    /// APT (Debian/Ubuntu).
    Apt,
}

impl Platform {
    /// Detect the current platform.
    ///
    /// - macOS: detected via `std::env::consts::OS == "macos"`
    /// - Arch: `/etc/os-release` ID is arch, endeavouros, garuda, cachyos, or artix
    /// - Debian: all other Linux (fallback)
    ///
    /// # Errors
    ///
    /// Returns [`PlatformError::UnsupportedOs`] on unsupported operating systems
    /// (e.g., Windows).
    ///
    /// Returns [`PlatformError::OsReleaseRead`] if `/etc/os-release` cannot be read
    /// on Linux.
    pub fn detect() -> Result<Self, PlatformError> {
        match std::env::consts::OS {
            "macos" => Ok(Self::MacOS),
            "linux" => {
                let content = std::fs::read_to_string("/etc/os-release")
                    .map_err(|e| PlatformError::OsReleaseRead { source: e })?;
                let id = parse_os_release_id(&content).unwrap_or_default();
                Ok(classify_linux_distro(&id))
            }
            other => Err(PlatformError::UnsupportedOs {
                os: (*other).to_owned(),
            }),
        }
    }

    /// Return the primary package manager for this platform.
    #[must_use]
    pub fn package_manager(self) -> PackageManager {
        match self {
            Self::MacOS => PackageManager::Brew,
            Self::Arch => PackageManager::Pacman,
            Self::Debian => PackageManager::Apt,
        }
    }

    /// Human-readable display name.
    #[must_use]
    pub fn display_name(self) -> &'static str {
        match self {
            Self::MacOS => "macOS",
            Self::Arch => "Arch Linux",
            Self::Debian => "Debian/Ubuntu",
        }
    }
}

impl fmt::Display for Platform {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.display_name())
    }
}

impl PackageManager {
    /// Build the install command for a package.
    ///
    /// Returns the full command as a `Vec` of strings (e.g., `["brew", "install", "tmux"]`).
    #[must_use]
    pub fn install_cmd(self, package: &str) -> Vec<String> {
        match self {
            Self::Brew => vec!["brew".to_owned(), "install".to_owned(), package.to_owned()],
            Self::Pacman => vec![
                "sudo".to_owned(),
                "pacman".to_owned(),
                "-S".to_owned(),
                "--noconfirm".to_owned(),
                package.to_owned(),
            ],
            Self::Apt => vec![
                "sudo".to_owned(),
                "apt-get".to_owned(),
                "install".to_owned(),
                "-y".to_owned(),
                package.to_owned(),
            ],
        }
    }

    /// Map a binary name to the package name for this package manager.
    ///
    /// Some binaries have different package names across platforms
    /// (e.g., `node` is `nodejs` on apt, `node` on brew/pacman).
    #[must_use]
    pub fn package_name(self, binary: &str) -> &'static str {
        match (self, binary) {
            (_, "node") => "nodejs",
            (Self::Pacman, "gh") => "github-cli",
            (_, "gh") => "gh",
            (_, "git") => "git",
            (_, "tmux") => "tmux",
            (_, "zellij") => "zellij",
            (_, "fzf") => "fzf",
            (_, "claude") => "claude",
            (_, "pi") => "pi",
            (_, "codex") => "codex",
            (_, "gemini") => "gemini",
            (_, "amp") => "amp",
            _ => "unknown",
        }
    }
}

impl fmt::Display for PackageManager {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Brew => f.write_str("brew"),
            Self::Pacman => f.write_str("pacman"),
            Self::Apt => f.write_str("apt"),
        }
    }
}

/// Parse the `ID` field from os-release content.
///
/// Handles both unquoted (`ID=arch`) and quoted (`ID="linuxmint"`) values.
///
/// Returns `None` if the `ID` line is not found.
fn parse_os_release_id(content: &str) -> Option<String> {
    for line in content.lines() {
        let trimmed = line.trim();
        if let Some(value) = trimmed.strip_prefix("ID=") {
            // Strip surrounding quotes if present.
            let unquoted = value.trim_matches('"').trim_matches('\'');
            return Some(unquoted.to_lowercase());
        }
    }
    None
}

/// Classify a Linux distribution ID into a [`Platform`].
///
/// Known Arch-based IDs: `arch`, `endeavouros`, `garuda`, `cachyos`, `artix`.
/// All other IDs fall back to [`Platform::Debian`].
fn classify_linux_distro(id: &str) -> Platform {
    match id {
        "arch" | "endeavouros" | "garuda" | "cachyos" | "artix" => Platform::Arch,
        _ => Platform::Debian,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── parse_os_release_id tests ─────────────────────────────────────────

    #[test]
    fn test_parse_os_release_id_arch() {
        let content = "NAME=\"Arch Linux\"\nID=arch\nPRETTY_NAME=\"Arch Linux\"\n";
        assert_eq!(parse_os_release_id(content), Some("arch".to_owned()));
    }

    #[test]
    fn test_parse_os_release_id_ubuntu() {
        let content = "NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"24.04\"\n";
        assert_eq!(parse_os_release_id(content), Some("ubuntu".to_owned()));
    }

    #[test]
    fn test_parse_os_release_id_endeavouros() {
        let content = "NAME=\"EndeavourOS\"\nID=endeavouros\nID_LIKE=arch\n";
        assert_eq!(parse_os_release_id(content), Some("endeavouros".to_owned()));
    }

    #[test]
    fn test_parse_os_release_id_missing() {
        let content = "NAME=\"Some OS\"\nVERSION_ID=\"1.0\"\n";
        assert_eq!(parse_os_release_id(content), None);
    }

    #[test]
    fn test_parse_os_release_id_quoted() {
        let content = "NAME=\"Linux Mint\"\nID=\"linuxmint\"\nVERSION_ID=\"21.3\"\n";
        assert_eq!(parse_os_release_id(content), Some("linuxmint".to_owned()));
    }

    // ── classify_linux_distro tests ───────────────────────────────────────

    #[test]
    fn test_classify_arch() {
        assert_eq!(classify_linux_distro("arch"), Platform::Arch);
    }

    #[test]
    fn test_classify_endeavouros() {
        assert_eq!(classify_linux_distro("endeavouros"), Platform::Arch);
    }

    #[test]
    fn test_classify_ubuntu() {
        assert_eq!(classify_linux_distro("ubuntu"), Platform::Debian);
    }

    #[test]
    fn test_classify_unknown() {
        assert_eq!(classify_linux_distro("gentoo"), Platform::Debian);
    }

    // ── Platform::package_manager tests ───────────────────────────────────

    #[test]
    fn test_package_manager_macos() {
        assert_eq!(Platform::MacOS.package_manager(), PackageManager::Brew);
    }

    #[test]
    fn test_package_manager_arch() {
        assert_eq!(Platform::Arch.package_manager(), PackageManager::Pacman);
    }

    #[test]
    fn test_package_manager_debian() {
        assert_eq!(Platform::Debian.package_manager(), PackageManager::Apt);
    }

    // ── display_name tests ────────────────────────────────────────────────

    #[test]
    fn test_display_name_macos() {
        assert_eq!(Platform::MacOS.display_name(), "macOS");
    }

    #[test]
    fn test_display_name_arch() {
        assert_eq!(Platform::Arch.display_name(), "Arch Linux");
    }

    #[test]
    fn test_display_name_debian() {
        assert_eq!(Platform::Debian.display_name(), "Debian/Ubuntu");
    }

    // ── PackageManager::install_cmd tests ──────────────────────────────────

    #[test]
    fn test_install_cmd_brew() {
        let cmd = PackageManager::Brew.install_cmd("tmux");
        assert_eq!(cmd, vec!["brew", "install", "tmux"]);
    }

    #[test]
    fn test_install_cmd_pacman() {
        let cmd = PackageManager::Pacman.install_cmd("tmux");
        assert_eq!(cmd, vec!["sudo", "pacman", "-S", "--noconfirm", "tmux"]);
    }

    #[test]
    fn test_install_cmd_apt() {
        let cmd = PackageManager::Apt.install_cmd("tmux");
        assert_eq!(cmd, vec!["sudo", "apt-get", "install", "-y", "tmux"]);
    }

    // ── PackageManager::package_name tests ───────────────────────────────

    #[test]
    fn test_package_name_node_apt() {
        assert_eq!(PackageManager::Apt.package_name("node"), "nodejs");
    }

    #[test]
    fn test_package_name_node_brew() {
        assert_eq!(PackageManager::Brew.package_name("node"), "nodejs");
    }

    #[test]
    fn test_package_name_gh_pacman() {
        assert_eq!(PackageManager::Pacman.package_name("gh"), "github-cli");
    }

    #[test]
    fn test_package_name_gh_brew() {
        assert_eq!(PackageManager::Brew.package_name("gh"), "gh");
    }

    #[test]
    fn test_package_name_git_all() {
        assert_eq!(PackageManager::Brew.package_name("git"), "git");
        assert_eq!(PackageManager::Pacman.package_name("git"), "git");
        assert_eq!(PackageManager::Apt.package_name("git"), "git");
    }

    // ── Platform::detect integration test ─────────────────────────────────

    #[test]
    fn test_detect_returns_valid_platform() {
        // On any supported platform, detect() should succeed.
        let platform = Platform::detect().expect("should detect current platform");
        // Just verify it's one of the valid variants.
        assert!(
            matches!(
                platform,
                Platform::MacOS | Platform::Arch | Platform::Debian
            ),
            "detected platform should be a valid variant"
        );
    }
}
