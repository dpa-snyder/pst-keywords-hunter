package scanner

import (
	"path/filepath"
	"strings"
	"time"
)

// FileType represents the types of email containers we can scan.
type FileType int

const (
	TypePST  FileType = iota // .pst - Outlook Personal Storage Table
	TypeOST                  // .ost - Outlook Offline Storage Table
	TypeEML                  // .eml - RFC-822 email message
	TypeMSG                  // .msg - Microsoft Outlook message (OLE2)
	TypeMBOX                 // .mbox / .mbx - Unix mailbox format
)

// String returns the display name for a file type.
func (ft FileType) String() string {
	switch ft {
	case TypePST:
		return "PST"
	case TypeOST:
		return "OST"
	case TypeEML:
		return "EML"
	case TypeMSG:
		return "MSG"
	case TypeMBOX:
		return "MBOX"
	default:
		return "UNKNOWN"
	}
}

// Extensions returns the file extensions associated with this type.
func (ft FileType) Extensions() []string {
	switch ft {
	case TypePST:
		return []string{".pst"}
	case TypeOST:
		return []string{".ost"}
	case TypeEML:
		return []string{".eml"}
	case TypeMSG:
		return []string{".msg"}
	case TypeMBOX:
		return []string{".mbox", ".mbx"}
	default:
		return nil
	}
}

// AllFileTypes returns all supported file types.
func AllFileTypes() []FileType {
	return []FileType{TypePST, TypeOST, TypeEML, TypeMSG, TypeMBOX}
}

// MatchesExtension checks if a filename matches this file type.
func (ft FileType) MatchesExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, e := range ft.Extensions() {
		if ext == e {
			return true
		}
	}
	return false
}

// EventType describes what happened during scanning.
type EventType int

const (
	EventDiscovery   EventType = iota // Initial file discovery counts
	EventFlagged                      // Flagged folder detected
	EventFileStart                    // Starting to process a source file
	EventExtracting                   // Extracting PST/OST/MBOX
	EventSearching                    // Searching for a keyword
	EventMatch                        // Keyword match found
	EventSearchDone                   // Done searching for a keyword
	EventUnknownDate                  // Match exported to unknown_date bucket
	EventSkipped                      // File or format skipped
	EventFileDone                     // Done processing a source file
	EventError                        // Error occurred
	EventComplete                     // All scanning complete
)

// Event is emitted by the scanner for progress tracking.
type Event struct {
	Type       EventType
	SourceFile string // Source file being processed
	SourceType FileType
	OutputDir  string           // Numbered output directory name
	Term       string           // Keyword (for match/search events)
	Message    string           // Human-readable log line
	Note       string           // Optional note or degradation reason
	FileNum    int              // Current file number
	TotalFiles int              // Total files to process
	Counts     map[FileType]int // File counts (for EventDiscovery)
	Flagged    []string         // Flagged folders (for EventDiscovery)
}

// Config holds all settings for a scan run.
type Config struct {
	SourceDir        string
	OutputDir        string
	Terms            []string
	RejectedKeywords []RejectedKeyword
	EnabledTypes     map[FileType]bool
	StartDate        *time.Time
	EndDate          *time.Time
	DryRun           bool // If true, estimate results without exporting matches
}

// DryRunReport holds the results of a dry run for report generation.
type DryRunReport struct {
	SourceDir               string
	OutputDir               string
	Terms                   []string
	FileCounts              map[FileType]int
	TotalFiles              int
	FlaggedDirs             []string
	FilesByType             map[FileType][]string
	HasReadPST              bool
	KeywordMatchCounts      map[string]int
	FilesScannedForKeywords int
	Timestamp               string
}

type RejectedKeyword struct {
	Requested  string `json:"requested"`
	Normalized string `json:"normalized"`
	Kept       string `json:"kept"`
	Reason     string `json:"reason"`
}

type ConflictGroup struct {
	Normalized string   `json:"normalized"`
	Options    []string `json:"options"`
}

type ManifestRow struct {
	RunTimestamp        string
	SourceType          string
	SourceContainerPath string
	SourceContainerDir  string
	MessageBaseName     string
	MessageDirPath      string
	Keyword             string
	KeywordDir          string
	HitLocations        string
	MessageRelativePath string
	OutputEMLPath       string
	OutputHeaderPath    string
	SizeBytes           int64
	Status              string
	Note                string
}

type RunSummary struct {
	RunTimestamp         string
	RunDir               string
	ReportPath           string
	ManifestPath         string
	ReviewManifestPath   string
	ManifestWorkbookPath string
	ReviewWorkbookPath   string
	ConfigPath           string
	LogPath              string
	Mode                 string
	SourceRootLabel      string
	OutputRootLabel      string
	Terms                []string
	RejectedKeywords     []RejectedKeyword
	StartDate            string
	EndDate              string
	HasDateFilter        bool
	FileCounts           map[FileType]int
	TotalFiles           int
	FlaggedDirs          []string
	HasReadPST           bool
	HasHighFidelityMSG   bool
	SkippedFormats       []string
	Warnings             []string
	Errors               []string
	KeywordHits          map[string]int
	UnknownDateHits      map[string]int
	HitsBySource         map[string]int
	HitsByType           map[string]int
	UnknownDateTotal     int
	SkippedTotal         int
	FilesScanned         int
	FilesByType          map[FileType][]string
	ManifestRows         []ManifestRow
}
