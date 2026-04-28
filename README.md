# Archival Keyword Toolkit

Archival Keyword Toolkit is a small desktop and CLI toolkit for keyword-driven archival review.

- `MailHog` searches email containers and message files.
- `FileHog` searches non-email files by filename and, where supported, extracted content.

The current active release target is macOS and Linux.

## Tools

### MailHog

`MailHog` is the email-focused tool. It scans:

- `.pst`
- `.ost`
- `.eml`
- `.msg`
- `.mbox`
- `.mbx`

It exports matched messages, manifests, reports, and logs into a run folder for review.

Every MailHog run now writes:

- a reviewer CSV with the core review fields only
- a technical CSV with the full audit/detail columns
- reviewer and technical XLSX workbooks formatted as tables for spreadsheet review
- a shorter Markdown run report that summarizes the run and points to the spreadsheet artifacts

### FileHog

`FileHog` is the non-email tool. It scans:

- filenames for all supported file types
- matching folder names as a fallback only when the folder contains no content-searchable files and no filename hits
- extracted text for direct-text formats
- extracted text from `.docx`, `.xlsx`, `.pptx`
- extracted text from `.pdf` when `pdftotext` is available
- extracted text from legacy Office and OpenDocument files when LibreOffice is available
- `.zip` archives as guarded containers: internal filenames are searched, supported internal file content is searched, and the original ZIP is copied once

Email/container formats are intentionally excluded from FileHog content processing.

ZIP archive scanning uses safety guardrails. FileHog does not open nested archives by default, rejects unsafe internal paths, records encrypted or unreadable members as skipped, and stops scanning a ZIP if member-count or uncompressed-size limits are reached. ZIP member hits appear in the manifest with `archive_path`, `archive_internal_path`, and `archive_status`.

Matched FileHog reports now track:

- matched keywords by document
- filename occurrence counts per keyword
- folder fallback occurrence counts per keyword
- content occurrence counts per keyword
- per-document filename, folder fallback, and content hit totals
- best-available document date, date source, and date status when a date range is supplied

Every FileHog run now writes two match CSVs automatically:

- a reviewer CSV with the core review fields only
- a technical CSV with the full audit/detail columns
- reviewer and technical XLSX workbooks formatted as tables for spreadsheet review

### CLI / TUI

`cmd/keyword-hunter-cli/` provides the terminal entrypoint for the email scanner backend. The TUI lives in `tui/`.

`cmd/filehog-cli/` provides a headless CLI for FileHog estimate and full runs.

Useful FileHog test controls:

- `--start-date YYYY-MM-DD` and `--end-date YYYY-MM-DD` apply the review date range.
- `--search-scope both` searches folder/file names and supported file content.
- `--search-scope paths` searches file names plus folder-name fallback only.
- `--search-scope content` searches supported file content only.
- `--max-matches N` stops after `N` matched files/folder fallbacks. Use `0` for no limit.
- `--max-content-mb N` skips content extraction for files larger than `N` MiB. Use `0` for no limit.
- `--max-zip-gb N` stops scanning inside each ZIP after `N` GiB of uncompressed entries. Default is `4`; accepted range is `1` to `30`.
- `--verbose` prints every file start/done event; normal CLI output is concise for large archives.
- Keyword files may be one keyword per line or CSV-style quoted terms on one line.

FileHog skips common OS-generated trash during traversal, including AppleDouble sidecars, `.DS_Store`, `Thumbs.db`, `desktop.ini`, `$RECYCLE.BIN`, `.Trashes`, `.Spotlight-V100`, and `.fseventsd`.

When a FileHog date range is supplied, FileHog now checks each file first. The best available document date is selected in this order: filesystem creation time, embedded document creation time, filesystem modified time, embedded document modified time, then folder year only as a fallback when no file-level date is available. Files confidently before the range are excluded. Files with missing dates or dates after the range are kept as `unknown` for review.

In the desktop app, running a prescan builds a hidden inventory snapshot for that source. That snapshot now carries both the file inventory and cached per-file date metadata, so the next scan or estimate against the same source can reuse that work instead of recomputing it during the run. Content still has to be extracted and searched during content scans.

## Repository Structure

Core runtime and app code:

- `mailhog/` - MailHog Wails desktop app
- `filehog/` - FileHog Wails desktop app
- `scanner/` - email scanning backend
- `filescanner/` - non-email scanning backend
- `cmd/keyword-hunter-cli/` - CLI entrypoint
- `cmd/filehog-cli/` - FileHog headless CLI entrypoint
- `tui/` - terminal UI for the email scanner

Operational packaging and distribution files:

- `scripts/install-commands.sh` - local install/build workflow for `mailhog` and `filehog`
- `scripts/release/linux/` - Linux release build helpers
- `packaging/linux/` - Linux desktop launcher templates and packaging assets
- `mailhog/build/` - MailHog Wails build assets and platform metadata
- `filehog/build/` - FileHog Wails build assets and platform metadata

Project docs and project state:

- `README.md` - primary project guide
- `docs/FILEHOG-USER-GUIDE.md` - FileHog operator guide
- `docs/REVIEWER-GUIDE.md` - reviewer guide
- `TODO.md` - active follow-up list
- `testing/` - organized test helpers and fixture generators
- `project-dashboard/` - static project dashboard

## Quick Start

These steps install the `mailhog` and `filehog` commands and build the desktop apps locally.

### 1. Get the repository

With Git:

```bash
git clone https://github.com/dpa-snyder/pst-keywords-hunter.git
cd pst-keywords-hunter
```

Without Git:

1. Open the repository on GitHub.
2. Click `Code`.
3. Click `Download ZIP`.
4. Unzip it.
5. Open a terminal in the extracted `pst-keywords-hunter` folder.

If you already have a clone:

```bash
cd pst-keywords-hunter
git pull
```

### 2. Install build prerequisites

Go `1.24` or newer is required.

macOS:

```bash
xcode-select --install
brew install go node
```

Ubuntu 24.04 and similar:

```bash
sudo apt update
sudo apt install -y nodejs npm build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

Ubuntu 22.04 / Debian variants using WebKit 4.0:

```bash
sudo apt update
sudo apt install -y nodejs npm build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.0-dev
```

If your Linux distro package for Go is older than `1.24`, install a newer Go release from [go.dev/dl](https://go.dev/dl/), then confirm with:

```bash
go version
```

### 3. Run the installer

```bash
./scripts/install-commands.sh
```

The installer:

- builds both frontends
- builds both desktop binaries
- prompts for an install directory for `mailhog` and `filehog`
- creates Linux desktop launchers when run on Linux

For scripted installs:

```bash
./scripts/install-commands.sh --bin-dir "$HOME/.local/bin"
```

### 4. Add the install directory to `PATH` if needed

If the installer says your selected directory is not on `PATH`, add it to your shell profile.

Example for `zsh`:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Example for `bash`:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### 5. Launch the apps

```bash
mailhog
filehog
```

On Linux, the installer also creates desktop launchers in `~/.local/share/applications/`.

## Runtime Dependencies

The apps can run without every optional helper installed, but some file formats rely on external tools.

MailHog:

- `readpst` for PST and OST extraction
- Python 3 plus `extract-msg` for `.msg`

FileHog:

- `pdftotext` for PDF text extraction
- `soffice --headless` for some legacy Office and OpenDocument formats

Dependency guidance inside the apps is platform-aware for macOS and Linux.

## Testing

The repository keeps unit tests in the Go packages and keeps tool-oriented helpers in `testing/`.

Useful commands:

```bash
go test ./...
./testing/run-all.sh
go run ./cmd/filehog-cli --estimate --source ./docs --output ./results --keywords-file terms.txt --start-date 2022-01-01 --end-date 2025-01-31
go run ./cmd/filehog-cli --estimate --search-scope paths --max-matches 10 --source ./docs --output ./results --keywords "Northwind,AgencyX,ExampleCorp,Example Corp"
```

The `testing/` tree is organized by tool:

- `testing/mailhog/`
- `testing/filehog/`

Each test area includes a fixture generator and a small runner script. Generated fixtures and outputs are ignored.

## Linux Packaging

Linux release support currently lives in:

- `scripts/release/linux/build-mailhog-linux.sh`
- `scripts/release/linux/build-filehog-linux.sh`
- `scripts/release/linux/build-linux-release.sh`

Desktop entry templates live in:

- `packaging/linux/mailhog/mailhog.desktop`
- `packaging/linux/filehog/filehog.desktop`

## Dashboard

`project-dashboard/` contains a lightweight static dashboard that summarizes:

- project overview
- tools and features
- architecture
- docs
- dependencies
- limitations
- open follow-up items
- recent cleanup changes

It is plain HTML, CSS, and JavaScript and reads from `project-dashboard/data/project-data.json`.

## Troubleshooting

### `fatal: not a git repository`

You ran `git pull` from the wrong folder. For first-time setup:

```bash
git clone https://github.com/dpa-snyder/pst-keywords-hunter.git
cd pst-keywords-hunter
./scripts/install-commands.sh
```

### Linux build fails with missing `webkit2gtk`

Install the Linux build prerequisites from the section above, then rerun the installer.

### Linux app launches with GTK or EGL warnings

Minor desktop-environment warnings can still appear even when the app works. Treat those as environment cleanup unless the app fails to launch or render.
