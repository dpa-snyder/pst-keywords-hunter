package tui

import (
	"fmt"
	"keyword-hunter/scanner"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Screen int

const (
	ScreenSetup Screen = iota
	ScreenKeywordConflicts
	ScreenScanning
	ScreenSummary
)

type SetupField int

const (
	FieldSourceDir SetupField = iota
	FieldOutputDir
	FieldKeywords
	FieldKeywordsFile
	FieldDateFilter
	FieldStartDate
	FieldEndDate
	FieldFileTypes
	FieldEstimateRun
	FieldStart
)

const maxLogLines = 15

type Model struct {
	screen Screen

	sourceInput    textinput.Model
	outputInput    textinput.Model
	keywordsInput  textinput.Model
	kwFileInput    textinput.Model
	startDateInput textinput.Model
	endDateInput   textinput.Model
	focusedField   SetupField

	fileTypes       []scanner.FileType
	fileTypeEnabled map[scanner.FileType]bool
	checkboxCursor  int
	dateFilter      bool
	dryRun          bool

	discovered         map[scanner.FileType]int
	flaggedDirs        []string
	discoveryDone      bool
	hasReadPST         bool
	hasHighFidelityMSG bool
	installingDeps     bool
	setupStatus        string

	pendingTerms     []string
	rejectedKeywords []scanner.RejectedKeyword
	conflicts        []scanner.ConflictGroup
	conflictIndex    int
	conflictCursor   int
	pendingStartDate *time.Time
	pendingEndDate   *time.Time
	pendingConfig    *scanner.Config

	events           chan scanner.Event
	done             chan error
	progress         progress.Model
	spinner          spinner.Model
	currentFile      string
	currentType      scanner.FileType
	currentKeyword   string
	fileNum          int
	totalFiles       int
	hitCounts        map[string]int
	totalHits        int
	skippedCount     int
	unknownDateCount int
	logLines         []string
	scanErr          error
	startTime        time.Time
	endTime          time.Time

	width  int
	height int
}

type scanEvent struct {
	event scanner.Event
}

type scanDoneMsg struct{ err error }

type discoverMsg struct {
	counts             map[scanner.FileType]int
	flagged            []string
	hasReadPST         bool
	hasHighFidelityMSG bool
}

type installDoneMsg struct {
	err error
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func NewModel() Model {
	makeInput := func(placeholder string, width int) textinput.Model {
		input := textinput.New()
		input.Placeholder = placeholder
		input.Width = width
		input.CharLimit = 500
		return input
	}

	si := makeInput("./source-root", 60)
	si.Focus()
	oi := makeInput("./output-root", 60)
	ki := makeInput("term1, term2, \"multi word term\"", 60)
	ki.CharLimit = 2000
	kf := makeInput("keywords.txt (optional, one per line)", 60)
	sd := makeInput("YYYY-MM-DD (optional)", 30)
	ed := makeInput("YYYY-MM-DD (optional)", 30)

	allTypes := scanner.AllFileTypes()
	enabled := make(map[scanner.FileType]bool)
	for _, ft := range allTypes {
		enabled[ft] = true
	}

	p := progress.New(progress.WithDefaultGradient())
	p.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	return Model{
		screen:             ScreenSetup,
		sourceInput:        si,
		outputInput:        oi,
		keywordsInput:      ki,
		kwFileInput:        kf,
		startDateInput:     sd,
		endDateInput:       ed,
		focusedField:       FieldSourceDir,
		fileTypes:          allTypes,
		fileTypeEnabled:    enabled,
		discovered:         make(map[scanner.FileType]int),
		hasReadPST:         scanner.HasReadPST(),
		hasHighFidelityMSG: scanner.HasHighFidelityMSG(),
		hitCounts:          make(map[string]int),
		progress:           p,
		spinner:            s,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 20
		if m.progress.Width > 80 {
			m.progress.Width = 80
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.screen == ScreenSummary {
				return m, tea.Quit
			}
		}
	}

	switch m.screen {
	case ScreenSetup:
		return m.updateSetup(msg)
	case ScreenKeywordConflicts:
		return m.updateKeywordConflicts(msg)
	case ScreenScanning:
		return m.updateScanning(msg)
	case ScreenSummary:
		return m.updateSummary(msg)
	}
	return m, nil
}

func (m Model) View() string {
	switch m.screen {
	case ScreenSetup:
		return m.viewSetup()
	case ScreenKeywordConflicts:
		return m.viewKeywordConflicts()
	case ScreenScanning:
		return m.viewScanning()
	case ScreenSummary:
		return m.viewSummary()
	}
	return ""
}

func (m Model) updateSetup(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case discoverMsg:
		m.discovered = msg.counts
		m.flaggedDirs = msg.flagged
		m.discoveryDone = true
		m.hasReadPST = msg.hasReadPST
		m.hasHighFidelityMSG = msg.hasHighFidelityMSG
		m.applyDiscoveredDefaults()
		return m, nil
	case installDoneMsg:
		m.installingDeps = false
		if msg.err != nil {
			m.setupStatus = fmt.Sprintf("Dependency install failed: %v", msg.err)
			return m, nil
		}
		m.hasReadPST = scanner.HasReadPST()
		m.hasHighFidelityMSG = scanner.HasHighFidelityMSG()
		m.setupStatus = "Dependency install completed successfully."
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.focusedField = (m.focusedField + 1) % (FieldStart + 1)
			m.updateFocus()
			return m, nil
		case "shift+tab", "up":
			if m.focusedField == 0 {
				m.focusedField = FieldStart
			} else {
				m.focusedField--
			}
			m.updateFocus()
			return m, nil
		case "enter":
			if m.focusedField == FieldStart {
				return m.startScan()
			}
			if m.focusedField == FieldSourceDir {
				return m, m.discoverCmd()
			}
			m.focusedField = (m.focusedField + 1) % (FieldStart + 1)
			m.updateFocus()
			return m, nil
		case " ":
			switch m.focusedField {
			case FieldFileTypes:
				ft := m.fileTypes[m.checkboxCursor]
				m.fileTypeEnabled[ft] = !m.fileTypeEnabled[ft]
				return m, nil
			case FieldEstimateRun:
				m.dryRun = !m.dryRun
				return m, nil
			case FieldDateFilter:
				m.dateFilter = !m.dateFilter
				return m, nil
			}
		case "j":
			if m.focusedField == FieldFileTypes && m.checkboxCursor < len(m.fileTypes)-1 {
				m.checkboxCursor++
				return m, nil
			}
		case "k":
			if m.focusedField == FieldFileTypes && m.checkboxCursor > 0 {
				m.checkboxCursor--
				return m, nil
			}
		case "i":
			if m.canInstallDependencies() && !m.installingDeps {
				m.installingDeps = true
				m.setupStatus = "Installing required dependencies..."
				return m, m.installDependenciesCmd()
			}
		}
	}

	var cmd tea.Cmd
	switch m.focusedField {
	case FieldSourceDir:
		m.sourceInput, cmd = m.sourceInput.Update(msg)
	case FieldOutputDir:
		m.outputInput, cmd = m.outputInput.Update(msg)
	case FieldKeywords:
		m.keywordsInput, cmd = m.keywordsInput.Update(msg)
	case FieldKeywordsFile:
		m.kwFileInput, cmd = m.kwFileInput.Update(msg)
	case FieldStartDate:
		m.startDateInput, cmd = m.startDateInput.Update(msg)
	case FieldEndDate:
		m.endDateInput, cmd = m.endDateInput.Update(msg)
	}
	return m, cmd
}

func (m Model) updateKeywordConflicts(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.conflicts) == 0 {
		return m.finalizeStartScan()
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.conflictCursor > 0 {
				m.conflictCursor--
			}
		case "down", "j":
			if m.conflictCursor < len(m.conflicts[m.conflictIndex].Options)-1 {
				m.conflictCursor++
			}
		case "enter":
			conflict := m.conflicts[m.conflictIndex]
			keep := conflict.Options[m.conflictCursor]
			resolved, rejected, err := scanner.ResolveKeywordConflict(m.pendingTerms, conflict, keep)
			if err != nil {
				m.scanErr = err
				m.screen = ScreenSummary
				return m, nil
			}
			m.pendingTerms = resolved
			m.rejectedKeywords = append(m.rejectedKeywords, rejected...)
			m.conflictIndex++
			m.conflictCursor = 0
			if m.conflictIndex >= len(m.conflicts) {
				m.conflicts = scanner.FindKeywordConflicts(m.pendingTerms)
				if len(m.conflicts) == 0 {
					return m.finalizeStartScan()
				}
				m.conflictIndex = 0
			}
		}
	}
	return m, nil
}

func (m *Model) updateFocus() {
	m.sourceInput.Blur()
	m.outputInput.Blur()
	m.keywordsInput.Blur()
	m.kwFileInput.Blur()
	m.startDateInput.Blur()
	m.endDateInput.Blur()

	switch m.focusedField {
	case FieldSourceDir:
		m.sourceInput.Focus()
	case FieldOutputDir:
		m.outputInput.Focus()
	case FieldKeywords:
		m.keywordsInput.Focus()
	case FieldKeywordsFile:
		m.kwFileInput.Focus()
	case FieldStartDate:
		m.startDateInput.Focus()
	case FieldEndDate:
		m.endDateInput.Focus()
	}
}

func (m *Model) applyDiscoveredDefaults() {
	for _, ft := range m.fileTypes {
		m.fileTypeEnabled[ft] = m.discovered[ft] > 0
	}
}

func (m Model) selectedNeedsReadPST() bool {
	return (m.fileTypeEnabled[scanner.TypePST] && m.discovered[scanner.TypePST] > 0) ||
		(m.fileTypeEnabled[scanner.TypeOST] && m.discovered[scanner.TypeOST] > 0)
}

func (m Model) selectedNeedsMSGSupport() bool {
	return m.fileTypeEnabled[scanner.TypeMSG] && m.discovered[scanner.TypeMSG] > 0
}

func (m Model) canInstallDependencies() bool {
	if !scanner.ReadPSTDependencyStatus().AutoInstall || !scanner.MSGDependencyStatus().AutoInstall {
		return false
	}
	return m.discoveryDone && ((!m.hasReadPST && m.selectedNeedsReadPST()) || (!m.hasHighFidelityMSG && m.selectedNeedsMSGSupport()))
}

func (m Model) installDependenciesCmd() tea.Cmd {
	return func() tea.Msg {
		if !m.hasReadPST && m.selectedNeedsReadPST() {
			if err := scanner.InstallReadPST(); err != nil {
				return installDoneMsg{err: fmt.Errorf("readpst install failed. Run `%s` and try again. %w", scanner.ReadPSTDependencyStatus().InstallHint, err)}
			}
		}
		if !m.hasHighFidelityMSG && m.selectedNeedsMSGSupport() {
			if err := scanner.InstallHighFidelityMSG(); err != nil {
				return installDoneMsg{err: fmt.Errorf("MSG support install failed. Run `%s` and try again. %w", scanner.MSGDependencyStatus().InstallHint, err)}
			}
		}
		return installDoneMsg{}
	}
}

func (m Model) discoverCmd() tea.Cmd {
	srcDir := m.sourceInput.Value()
	if srcDir == "" {
		return nil
	}
	return func() tea.Msg {
		files, _ := scanner.DiscoverFiles(srcDir, map[scanner.FileType]bool{
			scanner.TypePST: true, scanner.TypeOST: true,
			scanner.TypeEML: true, scanner.TypeMSG: true,
			scanner.TypeMBOX: true,
		})
		counts := scanner.CountFiles(files)
		flagged, _ := scanner.ScanFlaggedFolders(srcDir)
		relFlagged := make([]string, 0, len(flagged))
		for _, dir := range flagged {
			relFlagged = append(relFlagged, scanner.RelPath(srcDir, dir))
		}
		return discoverMsg{
			counts:             counts,
			flagged:            relFlagged,
			hasReadPST:         scanner.HasReadPST(),
			hasHighFidelityMSG: scanner.HasHighFidelityMSG(),
		}
	}
}

func (m Model) viewSetup() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ Keyword Hunter"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("  Review-oriented scanning with estimate runs, date filtering, and root-relative outputs"))
	b.WriteString("\n\n")

	writeInlineField := func(field SetupField, label, value string) {
		prefix := "  "
		labelStyle := normalStyle
		if m.focusedField == field {
			prefix = "▸ "
			labelStyle = activeStyle
		}
		b.WriteString(prefix)
		b.WriteString(labelStyle.Render(label))
		b.WriteString(" ")
		b.WriteString(value)
		b.WriteString("\n")
	}

	writeInlineField(FieldSourceDir, "Source:", m.sourceInput.View())
	writeInlineField(FieldOutputDir, "Output:", m.outputInput.View())
	writeInlineField(FieldKeywords, "Keywords:", m.keywordsInput.View())
	writeInlineField(FieldKeywordsFile, "Keywords File:", m.kwFileInput.View())
	b.WriteString("\n")

	cursor := noCursorStyle.String()
	if m.focusedField == FieldDateFilter {
		cursor = cursorStyle.String()
	}
	check := checkboxUnchecked.String()
	if m.dateFilter {
		check = checkboxChecked.String()
	}
	b.WriteString(fmt.Sprintf("    %s%s Date Filter", cursor, check))
	b.WriteString(mutedStyle.Render(" (message date only; start only, end only, or both)"))
	b.WriteString("\n")
	writeInlineField(FieldStartDate, "Start:", m.startDateInput.View())
	writeInlineField(FieldEndDate, "End:", m.endDateInput.View())
	b.WriteString("\n")

	if m.focusedField == FieldFileTypes {
		b.WriteString(activeStyle.Render("▸ File Types:"))
	} else {
		b.WriteString("  File Types:")
	}
	b.WriteString("\n")
	if m.discoveryDone {
		b.WriteString("    " + mutedStyle.Render("Prescan complete; detected types are pre-checked.") + "\n")
	} else {
		b.WriteString("    " + mutedStyle.Render("Press enter on the source field to prescan for mail types.") + "\n")
	}
	for i, ft := range m.fileTypes {
		itemCursor := noCursorStyle.String()
		if m.focusedField == FieldFileTypes && i == m.checkboxCursor {
			itemCursor = cursorStyle.String()
		}
		itemCheck := checkboxUnchecked.String()
		if m.fileTypeEnabled[ft] {
			itemCheck = checkboxChecked.String()
		}
		ext := strings.Join(ft.Extensions(), ", ")
		countStr := ""
		if m.discoveryDone {
			countStr = mutedStyle.Render(fmt.Sprintf(" %d found", m.discovered[ft]))
		}
		b.WriteString(fmt.Sprintf("    %s%s %s %s%s\n", itemCursor, itemCheck, ft.String(), mutedStyle.Render("("+ext+")"), countStr))
	}
	b.WriteString("\n")

	if m.canInstallDependencies() {
		msgs := make([]string, 0, 2)
		if !m.hasReadPST && m.selectedNeedsReadPST() {
			msgs = append(msgs, "PST/OST requires readpst")
		}
		if !m.hasHighFidelityMSG && m.selectedNeedsMSGSupport() {
			msgs = append(msgs, "MSG requires extract-msg")
		}
		message := strings.Join(msgs, " • ")
		if m.installingDeps {
			message = "Installing required dependencies now..."
		} else {
			message += ". Press i to install, or uncheck those formats to continue without them."
		}
		b.WriteString(warningStyle.Render("  Dependency: " + message))
		b.WriteString("\n\n")
	}

	if m.setupStatus != "" {
		style := mutedStyle
		status := strings.ToLower(m.setupStatus)
		if strings.Contains(status, "failed") {
			style = errorStyle
		} else if strings.Contains(status, "completed") || strings.Contains(status, "success") {
			style = successStyle
		}
		b.WriteString(style.Render("  " + m.setupStatus))
		b.WriteString("\n\n")
	}

	if len(m.flaggedDirs) > 0 {
		b.WriteString(flaggedStyle.Render(fmt.Sprintf("  %d flagged folder(s) detected; details will go into the run report.", len(m.flaggedDirs))))
		b.WriteString("\n\n")
	}

	cursor = noCursorStyle.String()
	if m.focusedField == FieldEstimateRun {
		cursor = cursorStyle.String()
	}
	check = checkboxUnchecked.String()
	if m.dryRun {
		check = checkboxChecked.String()
	}
	b.WriteString(fmt.Sprintf("    %s%s Estimate Run", cursor, check))
	b.WriteString(mutedStyle.Render(" (report/CSV/JSON only; no match export)"))
	b.WriteString("\n\n")

	startLabel := "START SCAN"
	if m.dryRun {
		startLabel = "RUN ESTIMATE"
	}
	if m.focusedField == FieldStart {
		b.WriteString(activeStyle.Render(fmt.Sprintf("  ▸ [ %s ]", startLabel)))
	} else {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("    [ %s ]", startLabel)))
	}
	b.WriteString("\n\n")

	help := "  tab/↑↓: navigate • space: toggle • enter: select/next • ctrl+c: quit"
	if m.canInstallDependencies() {
		help += " • i: install dependencies"
	}
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m Model) viewKeywordConflicts() string {
	var b strings.Builder
	conflict := m.conflicts[m.conflictIndex]

	b.WriteString(titleStyle.Render("Resolve Keyword Conflict"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Multiple requested terms normalize to the same folder: %s\n\n", activeStyle.Render(conflict.Normalized)))
	b.WriteString("  Choose the term to keep for this folder name:\n\n")
	for i, option := range conflict.Options {
		cursor := noCursorStyle.String()
		style := normalStyle
		if i == m.conflictCursor {
			cursor = cursorStyle.String()
			style = activeStyle
		}
		b.WriteString(fmt.Sprintf("    %s%s\n", cursor, style.Render(option)))
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  Conflict %d of %d. Rejected terms will be recorded in the run report.", m.conflictIndex+1, len(m.conflicts))))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("  ↑↓/j/k: move • enter: keep selected term"))
	return b.String()
}

func (m Model) startScan() (tea.Model, tea.Cmd) {
	typedTerms := splitKeywordInput(m.keywordsInput.Value())
	fileTerms := []string{}
	if m.kwFileInput.Value() != "" {
		loaded, err := scanner.LoadKeywordsFile(m.kwFileInput.Value())
		if err != nil {
			m.scanErr = fmt.Errorf("failed to load keywords file: %w", err)
			m.screen = ScreenSummary
			return m, nil
		}
		fileTerms = loaded
	}
	terms := scanner.MergeKeywordLists(typedTerms, fileTerms)
	if len(terms) == 0 && !m.dryRun {
		m.scanErr = fmt.Errorf("no keywords provided")
		m.screen = ScreenSummary
		return m, nil
	}
	if m.sourceInput.Value() == "" {
		m.scanErr = fmt.Errorf("source directory is required")
		m.screen = ScreenSummary
		return m, nil
	}
	if m.outputInput.Value() == "" {
		m.scanErr = fmt.Errorf("output directory is required")
		m.screen = ScreenSummary
		return m, nil
	}
	if err := scanner.ValidateRoots(m.sourceInput.Value(), m.outputInput.Value()); err != nil {
		m.scanErr = err
		m.screen = ScreenSummary
		return m, nil
	}

	var startDate, endDate *time.Time
	var err error
	if m.dateFilter {
		startDate, err = scanner.ParseDateInput(m.startDateInput.Value())
		if err != nil {
			m.scanErr = fmt.Errorf("invalid start date: %w", err)
			m.screen = ScreenSummary
			return m, nil
		}
		endDate, err = scanner.ParseDateInput(m.endDateInput.Value())
		if err != nil {
			m.scanErr = fmt.Errorf("invalid end date: %w", err)
			m.screen = ScreenSummary
			return m, nil
		}
		if startDate != nil && endDate != nil && startDate.After(*endDate) {
			m.scanErr = fmt.Errorf("start date must not be after end date")
			m.screen = ScreenSummary
			return m, nil
		}
	}

	if m.canInstallDependencies() {
		m.setupStatus = "Required dependencies are missing. Press i to install them, or uncheck those formats to continue without them."
		return m, nil
	}

	conflicts := scanner.FindKeywordConflicts(terms)
	if len(conflicts) > 0 {
		m.pendingTerms = terms
		m.rejectedKeywords = nil
		m.conflicts = conflicts
		m.conflictIndex = 0
		m.conflictCursor = 0
		m.pendingStartDate = startDate
		m.pendingEndDate = endDate
		m.screen = ScreenKeywordConflicts
		return m, nil
	}

	m.pendingTerms = terms
	m.rejectedKeywords = nil
	m.pendingStartDate = startDate
	m.pendingEndDate = endDate
	return m.finalizeStartScan()
}

func (m Model) finalizeStartScan() (tea.Model, tea.Cmd) {
	cfg := scanner.Config{
		SourceDir:        m.sourceInput.Value(),
		OutputDir:        m.outputInput.Value(),
		Terms:            append([]string(nil), m.pendingTerms...),
		RejectedKeywords: append([]scanner.RejectedKeyword(nil), m.rejectedKeywords...),
		EnabledTypes:     cloneEnabled(m.fileTypeEnabled),
		StartDate:        m.pendingStartDate,
		EndDate:          m.pendingEndDate,
		DryRun:           m.dryRun,
	}

	m.pendingConfig = &cfg
	m.screen = ScreenScanning
	m.startTime = time.Now()
	m.events = make(chan scanner.Event, 100)
	m.done = make(chan error, 1)
	m.hitCounts = make(map[string]int)
	for _, term := range cfg.Terms {
		m.hitCounts[term] = 0
	}
	m.totalHits = 0
	m.skippedCount = 0
	m.unknownDateCount = 0
	m.currentKeyword = ""
	m.currentFile = ""
	m.logLines = nil
	m.scanErr = nil

	go func(cfg scanner.Config, ch chan scanner.Event, done chan error) {
		done <- scanner.Run(cfg, ch)
	}(cfg, m.events, m.done)

	return m, tea.Batch(m.spinner.Tick, m.waitForEvent(), tickCmd())
}

func (m Model) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-m.events
		if !ok {
			return scanDoneMsg{err: <-m.done}
		}
		return scanEvent{event: event}
	}
}

func (m Model) updateScanning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scanEvent:
		e := msg.event
		switch e.Type {
		case scanner.EventFileStart:
			m.currentFile = e.SourceFile
			m.currentType = e.SourceType
			m.fileNum = e.FileNum
			m.totalFiles = e.TotalFiles
		case scanner.EventSearching:
			m.currentKeyword = e.Term
		case scanner.EventMatch:
			m.hitCounts[e.Term]++
			m.totalHits++
		case scanner.EventUnknownDate:
			m.hitCounts[e.Term]++
			m.totalHits++
			m.unknownDateCount++
		case scanner.EventSkipped, scanner.EventError:
			m.skippedCount++
		case scanner.EventDiscovery:
			m.totalFiles = e.TotalFiles
		}
		if e.Message != "" {
			m.logLines = append(m.logLines, e.Message)
			if len(m.logLines) > maxLogLines {
				m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
			}
		}
		if e.Type == scanner.EventComplete {
			m.endTime = time.Now()
			m.screen = ScreenSummary
			return m, nil
		}
		return m, m.waitForEvent()
	case scanDoneMsg:
		m.endTime = time.Now()
		m.scanErr = msg.err
		m.screen = ScreenSummary
		return m, nil
	case tickMsg:
		return m, tickCmd()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) viewScanning() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ Keyword Hunter — Running"))
	b.WriteString("\n\n")

	pct := 0.0
	if m.totalFiles > 0 {
		pct = float64(m.fileNum) / float64(m.totalFiles)
	}
	pctDisplay := fmt.Sprintf("%.0f%%", pct*100)
	b.WriteString(fmt.Sprintf("  %s  %s  %d / %d files\n", m.progress.ViewAs(pct), progressLabelStyle.Render(pctDisplay), m.fileNum, m.totalFiles))

	elapsed := time.Since(m.startTime).Round(time.Second)
	etaStr := ""
	if m.fileNum > 0 && m.totalFiles > 0 && m.fileNum < m.totalFiles {
		avgPerFile := time.Since(m.startTime) / time.Duration(m.fileNum)
		remaining := avgPerFile * time.Duration(m.totalFiles-m.fileNum)
		etaStr = fmt.Sprintf("  •  ETA: %s", remaining.Round(time.Second))
	} else if m.fileNum >= m.totalFiles && m.totalFiles > 0 {
		etaStr = "  •  Finishing up..."
	}
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  Elapsed: %s%s", elapsed, etaStr)))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  %s  Current file: %s\n", m.spinner.View(), truncatePath(m.currentFile, 80)))
	if m.currentKeyword != "" {
		b.WriteString(fmt.Sprintf("  Current keyword: %s\n", activeStyle.Render(m.currentKeyword)))
	}
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("  Live Counters"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("    Total hits: %s\n", matchStyle.Render(fmt.Sprintf("%d", m.totalHits))))
	b.WriteString(fmt.Sprintf("    Unknown-date hits: %s\n", warningStyle.Render(fmt.Sprintf("%d", m.unknownDateCount))))
	b.WriteString(fmt.Sprintf("    Skipped count: %s\n\n", mutedStyle.Render(fmt.Sprintf("%d", m.skippedCount))))

	b.WriteString(sectionStyle.Render("  Keyword Hits"))
	b.WriteString("\n")
	for term, count := range m.hitCounts {
		style := mutedStyle
		if count > 0 {
			style = matchStyle
		}
		b.WriteString(style.Render(fmt.Sprintf("    %-30s %d", term, count)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("  Log"))
	b.WriteString("\n")
	for _, line := range m.logLines {
		b.WriteString(mutedStyle.Render("    " + truncate(line, 100)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  ctrl+c: abort"))
	return b.String()
}

func (m Model) updateSummary(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m Model) viewSummary() string {
	var b strings.Builder

	title := "⚡ Keyword Hunter — Complete"
	if m.dryRun {
		title = "⚡ Keyword Hunter — Estimate Complete"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	if m.scanErr != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %s", m.scanErr)))
		b.WriteString("\n\n")
	}

	elapsed := m.endTime.Sub(m.startTime).Round(time.Second)
	b.WriteString(fmt.Sprintf("  Files processed:   %s\n", successStyle.Render(fmt.Sprintf("%d", m.totalFiles))))
	b.WriteString(fmt.Sprintf("  Total hits:        %s\n", matchStyle.Render(fmt.Sprintf("%d", m.totalHits))))
	b.WriteString(fmt.Sprintf("  Unknown-date hits: %s\n", warningStyle.Render(fmt.Sprintf("%d", m.unknownDateCount))))
	b.WriteString(fmt.Sprintf("  Skipped count:     %s\n", mutedStyle.Render(fmt.Sprintf("%d", m.skippedCount))))
	b.WriteString(fmt.Sprintf("  Duration:          %s\n", mutedStyle.Render(elapsed.String())))
	b.WriteString(fmt.Sprintf("  Output root:       %s\n", mutedStyle.Render(filepath.Base(m.outputInput.Value()))))
	b.WriteString("\n")

	b.WriteString(sectionStyle.Render("  Hits by Keyword"))
	b.WriteString("\n")
	for term, count := range m.hitCounts {
		bar := strings.Repeat("█", min(count, 40))
		b.WriteString(fmt.Sprintf("    %-25s %4d  %s\n", term, count, successStyle.Render(bar)))
	}
	b.WriteString("\n")

	if len(m.rejectedKeywords) > 0 {
		b.WriteString(sectionStyle.Render("  Rejected Duplicate Keywords"))
		b.WriteString("\n")
		for _, rejected := range m.rejectedKeywords {
			b.WriteString(fmt.Sprintf("    %s rejected; kept %s for %s\n", rejected.Requested, rejected.Kept, rejected.Normalized))
		}
		b.WriteString("\n")
	}

	if len(m.flaggedDirs) > 0 {
		b.WriteString(flaggedStyle.Render(fmt.Sprintf("  %d flagged folder(s) were recorded in flagged_folders.txt", len(m.flaggedDirs))))
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("  q: quit"))
	return b.String()
}

func splitKeywordInput(value string) []string {
	return scanner.ParseInlineKeywords(value)
}

func cloneEnabled(src map[scanner.FileType]bool) map[scanner.FileType]bool {
	dst := make(map[scanner.FileType]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func truncatePath(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	parts := strings.Split(s, "/")
	result := s
	for i := 1; i < len(parts); i++ {
		result = ".../" + strings.Join(parts[i:], "/")
		if len(result) <= maxLen {
			return result
		}
	}
	return truncate(s, maxLen)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
