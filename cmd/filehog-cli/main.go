package main

import (
	"flag"
	"fmt"
	"keyword-hunter/filescanner"
	"keyword-hunter/scanner"
	"os"
	"strconv"
	"strings"
)

func main() {
	headless := flag.Bool("headless", false, "Accepted for compatibility; the FileHog CLI always runs headless")
	estimateRun := flag.Bool("estimate", false, "Estimate run: inspect files and report keyword occurrences without copying matches")
	sourceDir := flag.String("source", "", "Source directory containing non-email files")
	outputDir := flag.String("output", "", "Output directory for results")
	keywordsFlag := flag.String("keywords", "", "Comma-separated keywords")
	keywordsFile := flag.String("keywords-file", "", "Path to keywords file (one per line)")
	startDate := flag.String("start-date", "", "Optional inclusive start date in YYYY-MM-DD format")
	endDate := flag.String("end-date", "", "Optional inclusive end date in YYYY-MM-DD format")
	searchScopeFlag := flag.String("search-scope", string(filescanner.SearchScopeBoth), "Search scope: both, paths, or content")
	maxMatches := flag.Int("max-matches", 0, "Stop after this many matched files/folders; 0 means no limit")
	maxContentMB := flag.Int("max-content-mb", 0, "Skip content extraction for files larger than this many MiB; 0 means no limit")
	maxZipGB := flag.Int("max-zip-gb", filescanner.DefaultMaxZipGB, "Stop scanning inside a ZIP after this many uncompressed GiB; range 1-30")
	verbose := flag.Bool("verbose", false, "Print every file start/done event instead of concise progress")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `FileHog CLI — Scan non-email files for filename and content keyword matches

Usage:
  filehog-cli [options]

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  filehog-cli --estimate --source ./docs --output ./results --keywords-file terms.txt
  filehog-cli --estimate --search-scope paths --max-matches 10 --source ./docs --output ./results --keywords "northwind,infrastructure"
`)
	}

	flag.Parse()
	_ = headless

	run(*sourceDir, *outputDir, *keywordsFlag, *keywordsFile, *startDate, *endDate, *estimateRun, *searchScopeFlag, *maxMatches, *maxContentMB, *maxZipGB, *verbose)
}

func run(sourceDir, outputDir, keywordsFlag, keywordsFile, startDateValue, endDateValue string, dryRun bool, searchScopeValue string, maxMatches int, maxContentMB int, maxZipGB int, verbose bool) {
	if sourceDir == "" {
		fmt.Fprintln(os.Stderr, "Error: --source is required")
		os.Exit(1)
	}
	if outputDir == "" {
		fmt.Fprintln(os.Stderr, "Error: --output is required")
		os.Exit(1)
	}

	var typedTerms []string
	var fileTerms []string
	if keywordsFile != "" {
		loaded, err := scanner.LoadKeywordsFile(keywordsFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading keywords file: %v\n", err)
			os.Exit(1)
		}
		fileTerms = append(fileTerms, loaded...)
	}
	if keywordsFlag != "" {
		typedTerms = append(typedTerms, scanner.ParseInlineKeywords(keywordsFlag)...)
	}
	terms := scanner.MergeKeywordLists(typedTerms, fileTerms)
	if len(terms) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no keywords provided. Use --keywords or --keywords-file")
		os.Exit(1)
	}
	if conflicts := scanner.FindKeywordConflicts(terms); len(conflicts) > 0 {
		fmt.Fprintln(os.Stderr, "Error: keyword normalization conflicts detected in headless mode:")
		for _, conflict := range conflicts {
			fmt.Fprintf(os.Stderr, "  %s -> %s\n", conflict.Normalized, strings.Join(conflict.Options, ", "))
		}
		fmt.Fprintln(os.Stderr, "Resolve the conflicting terms and try again.")
		os.Exit(1)
	}

	startDate, err := scanner.ParseDateInput(startDateValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing --start-date: %v\n", err)
		os.Exit(1)
	}
	endDate, err := scanner.ParseDateInput(endDateValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing --end-date: %v\n", err)
		os.Exit(1)
	}
	if startDate != nil && endDate != nil && startDate.After(*endDate) {
		fmt.Fprintln(os.Stderr, "Error: start date must not be after end date")
		os.Exit(1)
	}
	normalizedScopeValue := strings.ToLower(strings.TrimSpace(searchScopeValue))
	searchScope := filescanner.NormalizeSearchScope(filescanner.SearchScope(normalizedScopeValue))
	if normalizedScopeValue != "" && string(searchScope) != normalizedScopeValue {
		fmt.Fprintf(os.Stderr, "Error: --search-scope must be one of both, paths, or content; got %q\n", searchScopeValue)
		os.Exit(1)
	}
	if maxMatches < 0 {
		fmt.Fprintf(os.Stderr, "Error: --max-matches must be 0 or greater; got %s\n", strconv.Itoa(maxMatches))
		os.Exit(1)
	}
	if maxContentMB < 0 {
		fmt.Fprintf(os.Stderr, "Error: --max-content-mb must be 0 or greater; got %s\n", strconv.Itoa(maxContentMB))
		os.Exit(1)
	}
	if maxZipGB < filescanner.MinMaxZipGB || maxZipGB > filescanner.MaxMaxZipGB {
		fmt.Fprintf(os.Stderr, "Error: --max-zip-gb must be between %d and %d; got %s\n", filescanner.MinMaxZipGB, filescanner.MaxMaxZipGB, strconv.Itoa(maxZipGB))
		os.Exit(1)
	}

	cfg := filescanner.Config{
		SourceDir:       sourceDir,
		OutputDir:       outputDir,
		Terms:           terms,
		StartDate:       startDate,
		EndDate:         endDate,
		DryRun:          dryRun,
		SearchScope:     searchScope,
		MaxMatches:      maxMatches,
		MaxContentBytes: int64(maxContentMB) * 1024 * 1024,
		MaxZipBytes:     filescanner.MaxZipBytesFromGB(maxZipGB),
	}

	events := make(chan filescanner.Event, 100)
	done := make(chan error, 1)

	go func() {
		done <- filescanner.Run(cfg, events)
	}()

	for event := range events {
		if shouldPrintEvent(event, verbose) {
			fmt.Println(event.Message)
		}
	}

	if err := <-done; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func shouldPrintEvent(event filescanner.Event, verbose bool) bool {
	if event.Message == "" {
		return false
	}
	if verbose {
		return true
	}
	switch event.Type {
	case filescanner.EventDiscovery, filescanner.EventMatch, filescanner.EventError, filescanner.EventComplete:
		return true
	case filescanner.EventFileStart:
		return event.FileNum > 0 && event.FileNum%1000 == 0
	default:
		return false
	}
}
