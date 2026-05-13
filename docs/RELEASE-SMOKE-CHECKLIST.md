# Release Smoke Checklist

Use this checklist before calling a macOS or Linux build ready for wider testing.

## 1. Preflight

Run these from the repository root:

```bash
go test ./...
./testing/run-all.sh
```

Expected result:

- all Go tests pass
- both fixture generators run cleanly
- no private/manual samples are required

## 2. Local Install

Run:

```bash
./scripts/install-commands.sh
```

Expected result:

- both frontends build
- both desktop binaries build
- `mailhog` and `filehog` are copied into the chosen bin directory
- on Linux, desktop launcher files are created

## 3. Command Launch Check

Open a new shell and run:

```bash
mailhog
filehog
```

Expected result:

- both apps launch
- no immediate crash dialog
- the main window appears and can be closed normally

## 4. MailHog Quick Scan

Generate safe fixtures:

```bash
./testing/mailhog/generate-fixtures.sh
```

Then use the GUI or CLI to scan:

- source: `testing/mailhog/generated/basic-eml`
- keywords: `Harbor`

Expected result:

- one match is found from `Inbox/hit.eml`
- reviewer and technical XLSX outputs are written
- the Markdown report is created

## 5. FileHog Quick Scan

Generate safe fixtures:

```bash
./testing/filehog/generate-fixtures.sh
```

Then use the GUI or CLI to scan:

- source: `testing/filehog/generated/basic-files`
- keywords: `harbor`

Expected result:

- one content hit from `docs/hit.txt`
- one filename hit from `docs/harbor-notes.bin`
- reviewer and technical XLSX outputs are written
- the Markdown report is created

## 6. Dependency Visibility

In the app checks area, confirm the expected runtime helpers are detected.

MailHog:

- `readpst`
- Python 3 / `extract-msg` when MSG support is needed

FileHog:

- `pdftotext`
- `soffice`

Expected result:

- installed dependencies show as available
- missing optional dependencies show a clear install hint instead of a crash

## 7. Linux-Specific Checks

On Linux only:

- confirm `mailhog` and `filehog` work from a new terminal session
- confirm the desktop launchers appear in the application menu
- click both launchers once

Expected result:

- launcher entries exist
- each launcher opens the expected app

## 8. macOS-Specific Checks

On macOS only:

- launch both commands from Terminal
- confirm Homebrew-installed helpers are detected even when the apps are launched from the GUI later

Expected result:

- dependencies from `/opt/homebrew/bin` or `/usr/local/bin` are recognized
- no false “dependency missing” banner when the helper is installed
