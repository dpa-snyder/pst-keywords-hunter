# FileHog User Guide

## Purpose

FileHog searches non-email files for keyword hits in:

- file names
- file content, when the file type is searchable
- folder names only as a fallback when the folder contains no content-searchable files and no matching file names

It is built for archival review where traceability matters as much as the hits.

## Basic Workflow

1. Choose a `Source Root`.
2. Choose an `Output Root`.
3. Enter keywords directly, load a keywords file, or use both.
4. Set a date range if the review is time-limited.
5. Optionally run `Prescan Source`.
6. Validate the run.
7. Start an estimate or a full scan.

## Prescan

Prescan is optional but useful on large archives.

It builds a hidden snapshot for the selected source that caches:

- the discovered file inventory
- file size and searchability classification
- per-file date metadata used by later date filtering

The next run against the same source can reuse that work.

## Search Scope

- `Names + content`: search file names, supported file content, and folder fallback rules
- `Names only`: search file names plus folder fallback rules
- `Content only`: search only supported file content

## Date Policy

When a date range is supplied, FileHog checks each file first and only uses folder year as a fallback.

Best available date hierarchy:

1. file created time
2. embedded document created time
3. file modified time
4. embedded document modified time
5. folder year fallback

Range handling:

- files confidently before the start date are excluded
- files with no usable date are kept as `unknown`
- files with a best available date after the end date are also kept as `unknown`

This policy is intentionally conservative so potentially relevant records are not dropped just because metadata is incomplete or later-added.

## Search Limits

- `Max Matched Items`: stop after a fixed number of matches
- `Max Content Size (MiB)`: skip content extraction for very large files
- `ZIP Safety Limit (GiB)`: stop scanning inside each ZIP after a configurable uncompressed-byte limit

## Output Package

Each run writes a timestamped folder containing:

- `matched_files/` with copied matched items
- `run_report_*.md`
- `review_manifest_*.csv`
- `review_manifest_*.xlsx`
- `match_manifest_*.csv`
- `match_manifest_*.xlsx`
- `inventory_*.csv`
- `run_config_*.json`
- `script_log.txt`

## Reviewer Output vs Technical Output

Use the reviewer manifest first.

- reviewer CSV/XLSX: cleaner spreadsheet for human review
- technical CSV/XLSX: fuller audit record with more implementation detail

## Supported Content Extraction

FileHog can search content in:

- direct-text files such as `.txt`, `.csv`, `.tsv`, `.json`, `.xml`, `.html`, `.log`
- OpenXML files such as `.docx`, `.xlsx`, `.pptx`
- PDFs through `pdftotext`
- legacy Office and OpenDocument files through `soffice`
- ZIP archives as guarded containers

## ZIP Behavior

ZIP files are treated as containers.

- internal file names are searched
- supported internal content is searched
- the original ZIP is copied once
- nested archives are not opened by default
- unsafe paths, encrypted members, and safety-limit cases are recorded rather than silently ignored

## Review Tips

- Start with the reviewer XLSX.
- Sort by `content_hit_total` when you want the strongest evidence first.
- Use `base_name` when you want to sort or group by filename only.
- Use `source_directory_path` to understand where a file came from in the source tree.
- Use `size_human` when you want quick readable size context and `size_bytes` when you need exact values.
- Use the technical manifest when you need implementation detail, fallback behavior, or deeper audit notes.
