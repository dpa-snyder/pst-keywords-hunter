# Testing Layout

This repository keeps package-level Go tests close to the code and keeps repeatable tool-oriented helpers in this `testing/` tree.

## Structure

- `testing/mailhog/` - MailHog fixture helpers and runner
- `testing/filehog/` - FileHog fixture helpers and runner
- `testing/run-all.sh` - convenience wrapper for both tool test areas

## Conventions

- Track scripts, test code, and small text fixtures in Git.
- Do not track generated fixtures, test outputs, or machine-local manual sample sets.
- Use `generated/`, `outputs/`, `tmp/`, and `manual-samples/` for ignored per-tool working folders.

## Typical Commands

Run all tool-oriented checks:

```bash
./testing/run-all.sh
```

Run just MailHog-oriented checks:

```bash
./testing/mailhog/run.sh
```

Run just FileHog-oriented checks:

```bash
./testing/filehog/run.sh
```
