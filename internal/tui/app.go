package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui/components"
	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	twoPaneMinWidth = 96
	paneGapCols     = 2
)

type sessionsLoadedMsg struct {
	sessions  map[string]session.Session
	userState session.UserState
	stateErr  error
	err       error
}

type sessionResumedMsg struct {
	err error
}

type layoutSpec struct {
	twoPane       bool
	boxH          int
	innerH        int
	listBoxW      int
	listInnerW    int
	previewBoxW   int
	previewInnerW int
}

// Model represents the root Bubble Tea application state.
type Model struct {
	cfg            config.Config
	keys           KeyMap
	theme          styles.Theme
	themeSignature string
	searchInput    textinput.Model
	searching      bool
	previewFocused bool
	showArchived   bool
	showHelp       bool
	helpOffset     int
	loading        bool
	errMsg         string

	sessions  map[string]session.Session
	userState session.UserState
	results   []session.SearchResult
	rows      []components.Row
	expanded  map[string]bool

	preview          viewport.Model
	previewSessionID string
	previewWidth     int
	artifactCache    map[string]session.SessionArtifacts

	cursor int
	offset int
	width  int
	height int
	now    func() time.Time
}

// New constructs the initial root TUI model.
func New(cfg config.Config) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "search by title, directory, or prompt..."
	ti.CharLimit = 200

	theme, sig := styles.LoadTheme()
	ti.PromptStyle = theme.SearchPrompt
	ti.PlaceholderStyle = theme.HeaderHint

	m := Model{
		cfg:            cfg,
		keys:           DefaultKeyMap(),
		theme:          theme,
		themeSignature: sig,
		searchInput:    ti,
		loading:        true,
		expanded:       make(map[string]bool),
		userState:      session.UserState{Sessions: make(map[string]session.StateOverride)},
		preview:        viewport.New(0, 0),
		artifactCache:  make(map[string]session.SessionArtifacts),
		now:            time.Now,
	}
	m.syncSearchInputWidth()
	m.syncPreviewViewport(true)
	return m
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd {
	return loadSessionsCmd(m.cfg)
}

// Update handles incoming Bubble Tea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		lay := m.layoutDimensions()
		m.syncSearchInputWidth()
		m.ensureCursorVisible(m.listHeight(lay.innerH))
		m.syncPreviewViewport(false)
		m.clampHelpOffset()
		return m, nil

	case sessionsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.syncPreviewViewport(true)
			return m, nil
		}
		if msg.stateErr != nil {
			m.errMsg = msg.stateErr.Error()
		} else {
			m.errMsg = ""
		}
		m.sessions = msg.sessions
		if msg.userState.Sessions != nil {
			m.userState = msg.userState
		} else {
			m.userState = session.UserState{Sessions: make(map[string]session.StateOverride)}
		}
		m.artifactCache = make(map[string]session.SessionArtifacts)
		m.refreshRows(true)
		m.syncPreviewViewport(true)
		return m, nil

	case tea.FocusMsg:
		m.refreshTheme()
		return m, nil

	case sessionResumedMsg:
		m.refreshTheme()
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		} else {
			m.errMsg = ""
		}
		return m, loadSessionsCmd(m.cfg)

	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		if m.showHelp {
			return m.updateHelp(msg)
		}
		if m.previewFocused {
			return m.updatePreviewFocus(msg)
		}
		return m.updateBrowse(msg)
	}

	if m.searching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.searching = false
		m.searchInput.Blur()
		m.searchInput.SetValue("")
		m.refreshRows(false)
		return m, nil
	case tea.KeyEnter:
		m.searching = false
		m.searchInput.Blur()
		return m.resumeSelected()
	case tea.KeyUp, tea.KeyCtrlP:
		m.moveCursor(-1)
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		m.moveCursor(1)
		return m, nil
	}

	prev := m.searchInput.Value()
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	if m.searchInput.Value() != prev {
		m.refreshRows(false)
	}
	return m, cmd
}

func (m Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	lay := m.layoutDimensions()
	page := max(1, m.listHeight(lay.innerH)-1)

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = true
		m.helpOffset = 0
		return m, nil
	case key.Matches(msg, m.keys.Search):
		m.searching = true
		m.errMsg = ""
		m.syncSearchInputWidth()
		cmd := m.searchInput.Focus()
		return m, cmd
	case key.Matches(msg, m.keys.Clear):
		if m.searchInput.Value() != "" {
			m.searchInput.SetValue("")
			m.refreshRows(false)
		}
	case key.Matches(msg, m.keys.TogglePreviewFocus):
		m.previewFocused = true
		m.syncPreviewViewport(false)
	case key.Matches(msg, m.keys.TogglePin):
		m.togglePin()
	case key.Matches(msg, m.keys.ToggleArchive):
		m.toggleArchive()
	case key.Matches(msg, m.keys.ToggleShowArchived):
		m.toggleShowArchived()
	case key.Matches(msg, m.keys.Up):
		m.moveCursor(-1)
	case key.Matches(msg, m.keys.Down):
		m.moveCursor(1)
	case key.Matches(msg, m.keys.PageUp):
		m.moveCursor(-page)
	case key.Matches(msg, m.keys.PageDown):
		m.moveCursor(page)
	case key.Matches(msg, m.keys.Top):
		m.setCursor(0)
	case key.Matches(msg, m.keys.Bottom):
		m.setCursor(len(m.rows) - 1)
	case key.Matches(msg, m.keys.ToggleSubs):
		m.toggleSubsessions()
	case key.Matches(msg, m.keys.Collapse):
		m.collapseSelected()
	case key.Matches(msg, m.keys.Confirm):
		return m.resumeSelected()
	}

	return m, nil
}

func (m Model) updatePreviewFocus(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = true
		m.helpOffset = 0
		return m, nil
	case key.Matches(msg, m.keys.TogglePreviewFocus), msg.Type == tea.KeyEsc:
		m.previewFocused = false
		return m, nil
	case key.Matches(msg, m.keys.Search):
		m.previewFocused = false
		m.searching = true
		m.errMsg = ""
		m.syncSearchInputWidth()
		cmd := m.searchInput.Focus()
		return m, cmd
	case key.Matches(msg, m.keys.TogglePin):
		m.togglePin()
	case key.Matches(msg, m.keys.ToggleArchive):
		m.toggleArchive()
	case key.Matches(msg, m.keys.ToggleShowArchived):
		m.toggleShowArchived()
	case key.Matches(msg, m.keys.Confirm):
		return m.resumeSelected()
	case key.Matches(msg, m.keys.Up):
		m.preview.LineUp(1)
	case key.Matches(msg, m.keys.Down):
		m.preview.LineDown(1)
	case key.Matches(msg, m.keys.PageUp):
		m.preview.HalfPageUp()
	case key.Matches(msg, m.keys.PageDown):
		m.preview.HalfPageDown()
	case key.Matches(msg, m.keys.Top):
		m.preview.GotoTop()
	case key.Matches(msg, m.keys.Bottom):
		m.preview.GotoBottom()
	}
	return m, nil
}

func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	entries := whichKeyEntries()
	lay := components.CalcWhichKeyLayout(m.width, m.height, len(entries))
	page := max(1, lay.BodyH-1)

	switch {
	case key.Matches(msg, m.keys.Help), key.Matches(msg, m.keys.Clear), key.Matches(msg, m.keys.Quit):
		m.showHelp = false
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.helpOffset--
	case key.Matches(msg, m.keys.Down):
		m.helpOffset++
	case key.Matches(msg, m.keys.PageUp):
		m.helpOffset -= page
	case key.Matches(msg, m.keys.PageDown):
		m.helpOffset += page
	case key.Matches(msg, m.keys.Top):
		m.helpOffset = 0
	case key.Matches(msg, m.keys.Bottom):
		m.helpOffset = len(entries)
	}
	m.clampHelpOffset()
	return m, nil
}

func (m *Model) clampHelpOffset() {
	entries := whichKeyEntries()
	lay := components.CalcWhichKeyLayout(m.width, m.height, len(entries))
	maxOffset := max(0, len(entries)-lay.BodyH)
	if m.helpOffset < 0 {
		m.helpOffset = 0
	}
	if m.helpOffset > maxOffset {
		m.helpOffset = maxOffset
	}
}

func (m Model) resumeSelected() (tea.Model, tea.Cmd) {
	selected, ok := m.selectedSession()
	if !ok {
		return m, nil
	}
	cmd, err := session.NewResumeCmd(selected.ID)
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	return m, tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return sessionResumedMsg{err: execErr}
	})
}

func (m Model) selectedSession() (session.Session, bool) {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return session.Session{}, false
	}
	return m.rows[m.cursor].Session, true
}

func (m *Model) selectedRootID() (string, bool) {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return "", false
	}
	row := m.rows[m.cursor]
	targetID := row.Session.ID
	if row.IsSub {
		targetID = row.ParentID
	}
	return targetID, targetID != ""
}

func (m *Model) togglePin() {
	targetID, ok := m.selectedRootID()
	if !ok {
		return
	}
	s, exists := m.sessions[targetID]
	if !exists {
		return
	}

	nextPinned := !s.Pinned
	s.Pinned = nextPinned
	m.sessions[targetID] = s
	m.userState.SetPinned(targetID, nextPinned)
	if err := session.SaveUserState(m.cfg.StateFile, m.userState); err != nil {
		m.errMsg = err.Error()
	} else {
		m.errMsg = ""
	}
	m.refreshRows(true)
	m.syncPreviewViewport(true)
}

func (m *Model) toggleArchive() {
	targetID, ok := m.selectedRootID()
	if !ok {
		return
	}
	s, exists := m.sessions[targetID]
	if !exists {
		return
	}

	nextArchived := !s.Archived
	s.Archived = nextArchived
	m.sessions[targetID] = s
	m.userState.SetArchived(targetID, nextArchived)
	if err := session.SaveUserState(m.cfg.StateFile, m.userState); err != nil {
		m.errMsg = err.Error()
	} else {
		m.errMsg = ""
	}
	m.refreshRows(true)
	m.syncPreviewViewport(true)
}

func (m *Model) toggleShowArchived() {
	m.showArchived = !m.showArchived
	m.refreshRows(true)
	m.syncPreviewViewport(true)
}

func (m *Model) toggleSubsessions() {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	row := m.rows[m.cursor]
	targetID := row.Session.ID
	if row.IsSub {
		targetID = row.ParentID
	} else if !row.HasSubs {
		return
	}

	m.expanded[targetID] = !m.expanded[targetID]
	m.rebuildRows(targetID, m.cursor)
}

func (m *Model) collapseSelected() {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	row := m.rows[m.cursor]
	targetID := row.Session.ID
	if row.IsSub {
		targetID = row.ParentID
	}
	if !m.expanded[targetID] {
		return
	}
	m.expanded[targetID] = false
	m.rebuildRows(targetID, m.cursor)
}

func (m *Model) refreshRows(preserveSelection bool) {
	selectedID := ""
	fallbackCursor := 0
	if preserveSelection && m.cursor >= 0 && m.cursor < len(m.rows) {
		selectedID = m.rows[m.cursor].Session.ID
		fallbackCursor = m.cursor
	}
	m.results = session.SearchWithOptions(m.sessions, m.searchInput.Value(), session.SearchOptions{
		IncludeArchived: m.showArchived,
	})
	m.syncSearchInputWidth()
	m.rebuildRows(selectedID, fallbackCursor)
}

func (m *Model) rebuildRows(targetID string, fallbackCursor int) {
	m.rows = components.BuildRows(m.results, m.expanded)
	if len(m.rows) == 0 {
		m.cursor = 0
		m.offset = 0
		m.syncPreviewViewport(false)
		return
	}

	newCursor := min(max(0, fallbackCursor), len(m.rows)-1)
	if targetID != "" {
		for i, r := range m.rows {
			if r.Session.ID == targetID {
				newCursor = i
				break
			}
		}
	}
	m.setCursor(newCursor)
}

func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		m.cursor = 0
		m.offset = 0
		m.syncPreviewViewport(false)
		return
	}
	m.setCursor(m.cursor + delta)
}

func (m *Model) setCursor(pos int) {
	if len(m.rows) == 0 {
		m.cursor = 0
		m.offset = 0
		m.syncPreviewViewport(false)
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= len(m.rows) {
		pos = len(m.rows) - 1
	}
	m.cursor = pos

	lay := m.layoutDimensions()
	m.ensureCursorVisible(m.listHeight(lay.innerH))
	m.syncPreviewViewport(false)
}

func (m *Model) ensureCursorVisible(viewH int) {
	if viewH <= 0 {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+viewH {
		m.offset = m.cursor - viewH + 1
	}
	maxOffset := max(0, len(m.rows)-viewH)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) refreshTheme() {
	sig := styles.CurrentThemeSignature()
	if sig == m.themeSignature {
		return
	}
	theme, nextSig := styles.LoadTheme()
	m.theme = theme
	m.themeSignature = nextSig
	m.searchInput.PromptStyle = theme.SearchPrompt
	m.searchInput.PlaceholderStyle = theme.HeaderHint
	m.syncPreviewViewport(true)
}

func (m *Model) syncPreviewViewport(force bool) {
	lay := m.layoutDimensions()
	bodyH := m.listHeight(lay.innerH)
	previewW := lay.previewInnerW

	m.preview.Width = previewW
	m.preview.Height = bodyH

	if m.loading {
		if force || m.previewSessionID != "__loading__" || m.previewWidth != previewW {
			m.previewSessionID = "__loading__"
			m.previewWidth = previewW
			m.preview.SetContent(m.theme.PreviewMuted.Render(components.TruncateWidth("  Loading preview...", previewW)))
			m.preview.GotoTop()
		}
		return
	}

	sel, ok := m.selectedSession()
	if !ok {
		if force || m.previewSessionID != "" || m.previewWidth != previewW {
			m.previewSessionID = ""
			m.previewWidth = previewW
			msgLines := components.WrapPlain("Select a session from the list to inspect details, prompts, and artifacts.", max(10, previewW-2))
			m.preview.SetContent(m.theme.PreviewMuted.Render(strings.Join(msgLines, "\n")))
			m.preview.GotoTop()
		}
		return
	}

	sameSession := m.previewSessionID == sel.ID
	if sameSession && m.previewWidth == previewW && !force {
		return
	}

	art := m.loadCachedArtifacts(sel)
	content := components.FormatPreview(sel, art, previewW, m.now(), m.theme)
	m.preview.SetContent(content)
	if !sameSession {
		m.preview.GotoTop()
	}
	m.previewSessionID = sel.ID
	m.previewWidth = previewW
}

func (m *Model) loadCachedArtifacts(s session.Session) session.SessionArtifacts {
	if s.ID == "" || s.Path == "" {
		return session.SessionArtifacts{}
	}
	if art, ok := m.artifactCache[s.ID]; ok {
		return art
	}
	art := session.LoadArtifacts(s.Path)
	m.artifactCache[s.ID] = art
	return art
}

// View renders the centered rectangular TUI modal(s) and any active floating overlay.
func (m Model) View() string {
	lay := m.layoutDimensions()

	var modal string
	if lay.twoPane {
		left := m.renderListBox(lay)
		right := m.renderPreviewBox(lay)
		gap := strings.Repeat(" ", paneGapCols)
		modal = lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
	} else if m.previewFocused {
		modal = m.renderPreviewBox(lay)
	} else {
		modal = m.renderListBox(lay)
	}

	base := modal
	if m.width > 0 && m.height > 0 {
		base = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}

	if !m.showHelp {
		return base
	}

	popup, _ := components.RenderWhichKey(whichKeyEntries(), m.helpOffset, m.width, m.height, m.theme)
	return components.OverlayBottomCenter(base, popup, m.width, m.height)
}

func (m Model) renderListBox(lay layoutSpec) string {
	innerW := lay.listInnerW
	innerH := lay.innerH
	listH := m.listHeight(innerH)

	lines := make([]string, 0, innerH)
	lines = append(lines, m.headerView(innerW))
	lines = append(lines, m.theme.Divider.Render(strings.Repeat("─", innerW)))

	now := m.now()
	if m.loading {
		lines = append(lines, m.theme.HeaderHint.Render(components.TruncateWidth("  Scanning sessions...", innerW)))
		for i := 1; i < listH; i++ {
			lines = append(lines, "")
		}
	} else if len(m.rows) == 0 {
		emptyMsg := "  No sessions found."
		if m.searchInput.Value() != "" {
			emptyMsg = "  No matching sessions found."
		} else if _, archivedTotal, _ := m.sessionCounts(); !m.showArchived && archivedTotal > 0 {
			emptyMsg = fmt.Sprintf("  All %d sessions archived (press A to show).", archivedTotal)
		}
		lines = append(lines, m.theme.HeaderHint.Render(components.TruncateWidth(emptyMsg, innerW)))
		for i := 1; i < listH; i++ {
			lines = append(lines, "")
		}
	} else {
		end := min(len(m.rows), m.offset+listH)
		for i := m.offset; i < end; i++ {
			lines = append(lines, components.RenderRow(m.rows[i], i == m.cursor, innerW, now, m.theme))
		}
		for i := end - m.offset; i < listH; i++ {
			lines = append(lines, "")
		}
	}

	lines = append(lines, m.footerView(innerW))

	content := strings.Join(lines, "\n")

	borderStyle := m.theme.ModalBorder
	if !m.showHelp && (m.searching || !m.previewFocused) {
		borderStyle = m.theme.ModalBorderActive
	}

	return borderStyle.
		Width(innerW + 4).
		Height(innerH + 2).
		MaxWidth(lay.listBoxW).
		MaxHeight(lay.boxH).
		Render(content)
}

func (m Model) renderPreviewBox(lay layoutSpec) string {
	innerW := lay.previewInnerW
	innerH := lay.innerH

	lines := make([]string, 0, innerH)
	lines = append(lines, m.previewHeaderView(innerW))
	lines = append(lines, m.theme.Divider.Render(strings.Repeat("─", innerW)))

	vp := m.preview
	vp.Width = innerW
	vp.Height = m.listHeight(innerH)
	lines = append(lines, vp.View())
	lines = append(lines, m.previewFooterView(innerW))

	content := strings.Join(lines, "\n")

	borderStyle := m.theme.ModalBorder
	if !m.showHelp && m.previewFocused && !m.searching {
		borderStyle = m.theme.ModalBorderActive
	}

	return borderStyle.
		Width(innerW + 4).
		Height(innerH + 2).
		MaxWidth(lay.previewBoxW).
		MaxHeight(lay.boxH).
		Render(content)
}

func (m Model) sessionCounts() (activeTotal, archivedTotal, visibleTotal int) {
	for _, s := range m.sessions {
		if s.ParentID != "" {
			continue
		}
		if s.Archived {
			archivedTotal++
		} else {
			activeTotal++
		}
	}
	visibleTotal = activeTotal
	if m.showArchived {
		visibleTotal += archivedTotal
	}
	return activeTotal, archivedTotal, visibleTotal
}

func (m Model) headerCounter() string {
	activeTotal, archivedTotal, visibleTotal := m.sessionCounts()
	if m.searchInput.Value() != "" {
		return fmt.Sprintf("%d/%d", len(m.results), visibleTotal)
	}
	if m.showArchived {
		return fmt.Sprintf("%d sessions (%d arch)", visibleTotal, archivedTotal)
	}
	if archivedTotal > 0 {
		return fmt.Sprintf("%d sessions (+%d arch)", activeTotal, archivedTotal)
	}
	return fmt.Sprintf("%d sessions", activeTotal)
}

func (m Model) headerLayout(innerW int) (counter string, maxLeft int, inputWidth int) {
	counter = m.headerCounter()
	counterW := lipgloss.Width(counter)
	if innerW < counterW+5 {
		counter = ""
		counterW = 0
	}

	maxLeft = max(1, innerW-counterW-1)
	if counterW == 0 {
		maxLeft = max(1, innerW)
	}

	promptW := lipgloss.Width(m.searchInput.Prompt)
	// Reserve Prompt and 1 cursor cell added by textinput.View().
	inputWidth = max(1, maxLeft-promptW-1)
	return counter, maxLeft, inputWidth
}

func (m *Model) syncSearchInputWidth() {
	_, _, innerW, _ := m.boxDimensions()
	_, _, inputW := m.headerLayout(innerW)
	m.searchInput.Width = inputW
}

func (m Model) headerView(innerW int) string {
	counter, maxLeft, inputW := m.headerLayout(innerW)

	var leftStyled string
	if m.searching {
		m.searchInput.Width = inputW
		leftStyled = m.searchInput.View()
		if lipgloss.Width(leftStyled) > maxLeft {
			leftStyled = lipgloss.NewStyle().MaxWidth(maxLeft).Render(leftStyled)
		}
	} else if q := m.searchInput.Value(); q != "" {
		leftStyled = m.theme.SearchPrompt.Render(components.TruncateWidth("/ "+q, maxLeft))
	} else {
		leftStyled = m.theme.HeaderHint.Render(components.TruncateWidth("/ search by title, prompt, or directory", maxLeft))
	}

	if counter == "" {
		return leftStyled
	}
	gap := max(1, innerW-lipgloss.Width(leftStyled)-lipgloss.Width(counter))
	return leftStyled + strings.Repeat(" ", gap) + m.theme.HeaderHint.Render(counter)
}

func (m Model) previewHeaderView(innerW int) string {
	title := "Preview"
	titleStyle := m.theme.HeaderHint
	if m.previewFocused {
		title = "Preview [scroll]"
		titleStyle = m.theme.SearchPrompt
	}

	pctText := ""
	if m.preview.TotalLineCount() > m.preview.Height && m.preview.Height > 0 {
		pct := int(math.Round(m.preview.ScrollPercent() * 100))
		pctText = fmt.Sprintf("%d%%", pct)
	} else if m.preview.TotalLineCount() > 0 && !m.loading {
		pctText = "all"
	}

	pctW := lipgloss.Width(pctText)
	if innerW < pctW+6 {
		pctText = ""
		pctW = 0
	}

	maxLeft := innerW
	if pctW > 0 {
		maxLeft = max(1, innerW-pctW-1)
	}
	left := titleStyle.Render(components.TruncateWidth(title, maxLeft))
	if pctText == "" {
		return left
	}
	gap := max(1, innerW-lipgloss.Width(left)-pctW)
	return left + strings.Repeat(" ", gap) + m.theme.PreviewMuted.Render(pctText)
}

func (m Model) footerView(innerW int) string {
	if m.errMsg != "" {
		return m.theme.ErrorText.Render(components.TruncateWidth("Error: "+m.errMsg, innerW))
	}

	var hint string
	if m.searching {
		hint = "↑↓ select · Enter open · Esc clear"
	} else if m.searchInput.Value() != "" {
		hint = "↑↓ select · Enter open · Esc clear · ? keys · q quit"
	} else {
		hint = "↑↓ select · / search · Enter open · ? keys · q quit"
	}
	return m.theme.Footer.Render(components.TruncateWidth(hint, innerW))
}

func (m Model) previewFooterView(innerW int) string {
	var hint string
	if m.previewFocused {
		hint = "↑↓/Pg scroll · Tab back · ? keys · q quit"
	} else {
		hint = "Tab focus preview · ? keys"
	}
	return m.theme.Footer.Render(components.TruncateWidth(hint, innerW))
}

func whichKeyEntries() []components.WhichKeyEntry {
	return []components.WhichKeyEntry{
		{Desc: "Move selection / scroll", Key: "↑/k · ↓/j"},
		{Desc: "Page up / down", Key: "PgUp / PgDn"},
		{Desc: "Jump to top / bottom", Key: "g / G"},
		{Desc: "Search sessions", Key: "/"},
		{Desc: "Expand / collapse subsessions", Key: "l / h"},
		{Desc: "Focus preview / back to list", Key: "Tab"},
		{Desc: "Pin / unpin session", Key: "p"},
		{Desc: "Archive / unarchive session", Key: "a"},
		{Desc: "Show / hide archived sessions", Key: "A"},
		{Desc: "Resume selected session", Key: "Enter"},
		{Desc: "Clear search / close overlay", Key: "Esc"},
		{Desc: "Toggle keybindings menu", Key: "?"},
		{Desc: "Quit", Key: "q / Ctrl+C"},
	}
}

func (m Model) layoutDimensions() layoutSpec {
	termW := m.width
	termH := m.height
	if termW <= 0 {
		termW = 100
	}
	if termH <= 0 {
		termH = 30
	}

	boxH := termH * 80 / 100
	minH := min(max(8, termH-2), 16)
	maxH := min(max(8, termH-2), 36)
	boxH = clampInt(boxH, minH, maxH)
	innerH := max(4, boxH-4)

	if termW < twoPaneMinWidth {
		minW := min(max(12, termW-2), 68)
		maxW := max(12, termW-2)
		boxW := clampInt(termW*88/100, minW, maxW)
		innerW := max(10, boxW-6)
		return layoutSpec{
			twoPane:       false,
			boxH:          boxH,
			innerH:        innerH,
			listBoxW:      boxW,
			listInnerW:    innerW,
			previewBoxW:   boxW,
			previewInnerW: innerW,
		}
	}

	totalMinW := min(max(24, termW-2), 90)
	totalMaxW := min(max(24, termW-2), 168)
	totalW := clampInt(termW*92/100, totalMinW, totalMaxW)

	availW := max(20, totalW-paneGapCols)
	listBoxW := availW * 52 / 100
	if listBoxW < 42 {
		listBoxW = 42
	}
	previewBoxW := availW - listBoxW
	if previewBoxW < 36 {
		previewBoxW = 36
		listBoxW = availW - previewBoxW
	}

	return layoutSpec{
		twoPane:       true,
		boxH:          boxH,
		innerH:        innerH,
		listBoxW:      listBoxW,
		listInnerW:    max(10, listBoxW-6),
		previewBoxW:   previewBoxW,
		previewInnerW: max(10, previewBoxW-6),
	}
}

func (m Model) boxDimensions() (boxW, boxH, innerW, innerH int) {
	lay := m.layoutDimensions()
	return lay.listBoxW, lay.boxH, lay.listInnerW, lay.innerH
}

func (m Model) listHeight(innerH int) int {
	// Reserve 1 line for header, 1 line for divider, 1 line for footer.
	return max(1, innerH-3)
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func loadSessionsCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		sessions, err := session.Collect(cfg)
		if err != nil {
			return sessionsLoadedMsg{err: err}
		}
		st, stateErr := session.LoadUserState(cfg.StateFile)
		return sessionsLoadedMsg{
			sessions:  sessions,
			userState: st,
			stateErr:  stateErr,
		}
	}
}
