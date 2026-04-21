# fake-update

A fake system update TUI that simulates a realistic OS update screen with progress bars, stage indicators, live metrics, and scrolling log output. Auto-detects your system's hardware and distro, or lets you spoof any OS.

Optionally uses OpenAI to generate context-aware log lines that reference your real hardware. Falls back to built-in logs if no API key is set or the connection fails.

![demo](https://img.shields.io/badge/TUI-ncurses-blue)

## Features

- Auto-detects OS, CPU, RAM, disk, architecture, hostname, power state
- Distro-aware stages and log lines (Arch/pacman, Debian/apt, Fedora/dnf, Void/xbps, NixOS, macOS, Windows, ChromeOS, and more)
- AI-generated log lines via OpenAI (`gpt-4o-mini`) with strict `daemon: message` formatting
- Unicode block-character progress bar, braille spinners, traveling spark borders
- Configurable duration, OS spoofing, and no-AI fallback mode

## Build

Requires Go 1.21+ and ncurses development headers.

```bash
# Debian/Ubuntu
sudo apt install libncurses-dev

# Arch
sudo pacman -S ncurses

# Void
sudo xbps-install -S ncurses-devel

# macOS (usually preinstalled)
brew install ncurses
```

```bash
go build -o fake-update .
```

## Usage

```bash
# Basic — run for 5 minutes
./fake-update -d 300

# Spoof macOS
./fake-update -d 600 -os macOS -version "Sonoma 14.5"

# Spoof Windows
./fake-update -d 3600 -os Windows -version "11 23H2"

# Disable AI logs
./fake-update -d 300 -no-ai
```

Press `q` to quit.

## Options

| Flag | Short | Description |
|------|-------|-------------|
| `-duration` | `-d` | Duration in seconds (required) |
| `-os` | | OS name (auto-detected) |
| `-version` | | OS version string (auto-detected) |
| `-device` | | Device name (auto-detected) |
| `-build` | | Build/revision number (auto-detected) |
| `-no-ai` | | Disable AI-generated log lines |

## AI Log Generation

Set your OpenAI API key to enable AI-generated logs:

```bash
export OPENAI_API_KEY="sk-..."
./fake-update -d 300
```

Or use a `.env` file (see `.env.example`):

```bash
export $(cat .env | xargs) && ./fake-update -d 300
```

If the key is missing or the API is unreachable, the program silently falls back to built-in distro-specific log lines.

## License

AGPL-3.0
