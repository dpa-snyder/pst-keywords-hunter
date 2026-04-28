# Reviewer Guide

## What This Package Is

A review package is a traceable export of non-email files that matched one or more review keywords.

The package is designed to balance:

- easier human review
- preservation of source context
- enough technical detail to explain how a hit was found

## Files To Start With

Use these in this order:

1. `run_report_*.md`
2. `review_manifest_*.xlsx`
3. `matched_files/`

The reviewer manifest is the main spreadsheet for sorting and filtering. The technical manifest exists as a deeper audit record when needed.

## Reviewer Manifest Columns

- `base_name`: filename only, separated for easier sorting/grouping
- `source_directory_path`: original parent folder relative to the chosen source root
- `copied_directory_path`: parent folder of the copied item inside the output package
- `extension`: file type
- `hit_summary`: keyword hits with category and count, for example `content:Northwind=2 ; filename:Budget=1`
- `filename_hit_total`: total filename hit occurrences on that row
- `folder_hit_total`: total folder-name fallback hit occurrences on that row
- `content_hit_total`: total content hit occurrences on that row
- `document_date`: best available date used for review filtering
- `document_date_source`: plain-English description of the date source used
- `date_status`: review status for the date decision
- `content_status`: plain-English summary of how the item was searched
- `archive_path`: original ZIP path when the row refers to a file inside an archive
- `archive_internal_path`: path of the matched file inside the archive
- `archive_status`: plain-English archive processing state
- `size_bytes`: raw file size in bytes
- `size_human`: human-readable file size for easier review
- `note`: short explanation of how the item matched and what date source was used

## How Rows Are Sorted

The reviewer manifest is sorted to bring stronger evidence forward:

1. higher `content_hit_total`
2. then higher `filename_hit_total`
3. then higher `folder_hit_total`
4. then source path context

This means content matches generally appear before filename-only noise.

## What The Search Status Labels Mean

- `content searched`: the file content was searched
- `filename match only`: the filename matched, but the file content was not searched for that type
- `archive member filename match`: a file inside a ZIP matched by filename, but its content was not searched for that type
- `folder-name fallback`: the folder name matched, and that folder had no content-searchable files and no matching file names
- `archive filename match`: the ZIP file itself matched by name

## Date Hierarchy

This process does not rely on folder organization first. It checks the file itself before using folder year.

Best available date hierarchy:

1. file created time
2. embedded document created time
3. file modified time
4. embedded document modified time
5. folder year fallback

## Why The Date Hierarchy Works This Way

The goal is to avoid excluding potentially relevant records just because a drive was organized by year folders or because metadata is incomplete.

This is why:

- file-level dates are preferred over folder naming
- folder year is only used when the file itself does not provide a usable date
- files clearly before the review range are excluded
- files with missing dates or later dates are retained as `unknown` rather than discarded

## Archive Rows

If a row includes `archive_path` and `archive_internal_path`, the hit came from inside a ZIP file.

Important points:

- the original ZIP is copied once
- multiple archive-member hits may point to the same copied ZIP
- the manifest preserves the internal path so the hit location is still reviewable

## Reviewer Manifest vs Technical Manifest

Use the reviewer manifest for ordinary review work.

Use the technical manifest when you need:

- lower-level search metadata
- raw status values
- filesystem and embedded date fields side by side
- fuller implementation notes

## Reading `hit_summary`

The `hit_summary` column combines category and count:

- `filename:Northwind=1` means the filename matched `Northwind` once
- `content:Jordan Northwind=3` means the file content matched `Jordan Northwind` three times
- `folder:REC=1` means a folder-name fallback hit on `REC`

## Practical Review Advice

- Start in the XLSX version unless you specifically need CSV.
- Filter for `content_hit_total > 0` to surface the strongest likely evidence.
- Use `base_name` when sorting by filename alone.
- Use `source_directory_path` when reviewing source context.
- Use `copied_directory_path` plus `base_name` to find the copied artifact inside the package.
