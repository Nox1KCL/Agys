package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/session"
	"github.com/Nox1KCL/Agys/internal/tui/components"
	"github.com/Nox1KCL/Agys/internal/tui/styles"
	"github.com/Nox1KCL/Agys/internal/utils"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	twoPaneMinWidth     = 96
	threePaneMinWidth   = 138
	paneGapCols         = 2
	usageRefreshTimeout = 12 * time.Second
)

type sessionsLoadedMsg struct {
	sessions  map[string]session.Session
	userState session.UserState
	stateErr  error
	err       error
}

type usageLoadedMsg struct {
	usage session.AccountUsage
	found bool
	err   error
}

type sessionResumedMsg struct {
	err error
}

type deleteConfirmState struct {
	mode           session.DeleteMode
	plan           session.DeletePlan
	parentTitle    string
	trashPath      string
	confirmFocused bool
}

type layoutSpec struct {
	twoPane       bool
	threePane     bool
	boxH          int
	innerH        int
	listBoxW      int
	listInnerW    int
	previewBoxW   int
	previewInnerW int
	accountBoxW   int
	accountInnerW int
}

// Options configures optional startup behavior for the root TUI model.
type Options struct {
	ShowArchived bool
	InitialQuery string
	ExitOnResume bool
}

// ResumeTarget identifies a session scheduled to replace the TUI process on exit.
type ResumeTarget struct {
	ID      string
	WorkDir string
}

// Model represents the root Bubble Tea application state.
type Model struct {
	cfg            config.Config
	keys           KeyMap
	theme          styles.Theme
	themeSignature string
	searchInput    textinput.Model
	searching      bool
	renameInput    textinput.Model
	renaming       bool
	renameTargetID string
	previewFocused bool
	accountFocused bool
	showArchived   bool
	exitOnResume   bool
	pendingResume  *ResumeTarget
	confirmDelete  *deleteConfirmState
	showHelp       bool
	helpOffset     int
	loading        bool
	errMsg         string

	sessions    map[string]session.Session
	searchIndex *session.SearchIndex
	userState   session.UserState
	results     []session.SearchResult
	rows        []components.Row
	expanded    map[string]bool

	preview          viewport.Model
	previewSessionID string
	previewWidth     int
	artifactCache    map[string]session.SessionArtifacts

	account           viewport.Model
	accountUsage      session.AccountUsage
	accountLoaded     bool
	accountRefreshing bool
	accountErr        string
	accountWidth      int
	accountVersion    int

	cursor int
	offset int
	width  int
	height int
	now    func() time.Time
}

// New constructs the initial root TUI model.
func New(cfg config.Config) Model {
	return NewWithOptions(cfg, Options{})
}

// NewWithOptions constructs the root TUI model with explicit runtime options.
func NewWithOptions(cfg config.Config, opts Options) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "search by title, directory, or prompt..."
	ti.CharLimit = 200
	if opts.InitialQuery != "" {
		ti.SetValue(opts.InitialQuery)
	}

	ri := textinput.New()
	ri.Prompt = "Rename: "
	ri.Placeholder = "session title (empty resets)"
	ri.CharLimit = 160

	theme, sig := styles.LoadTheme()
	ti.PromptStyle = theme.SearchPrompt
	ti.PlaceholderStyle = theme.HeaderHint
	ri.PromptStyle = theme.SearchPrompt
	ri.PlaceholderStyle = theme.HeaderHint

	m := Model{
		cfg:            cfg,
		keys:           DefaultKeyMap(),
		theme:          theme,
		themeSignature: sig,
		searchInput:    ti,
		renameInput:    ri,
		showArchived:   opts.ShowArchived,
		exitOnResume:   opts.ExitOnResume,
		loading:        true,
		expanded:       make(map[string]bool),
		userState:      session.UserState{Sessions: make(map[string]session.StateOverride)},
		preview:        viewport.New(0, 0),
		artifactCache:  make(map[string]session.SessionArtifacts),
		account:        viewport.New(0, 0),
		now:            time.Now,
	}
	if cached, ok, _ := session.LoadCachedAccountUsage(cfg, m.now()); ok {
		m.accountUsage = cached
		m.accountLoaded = true
		m.accountVersion = 1
	}
	m.syncHeaderInputWidths()
	m.syncPreviewViewport(true)
	m.syncAccountViewport(true)
	return m
}

// PendingResume returns the selected session target if the TUI quit to exec agy directly.
func (m Model) PendingResume() (ResumeTarget, bool) {
	if m.pendingResume == nil {
		return ResumeTarget{}, false
	}
	return *m.pendingResume, true
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadSessionsCmd(m.cfg),
		fetchAccountUsageCmd(m.cfg, false, m.now),
	)
}

// Update handles incoming Bubble Tea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		lay := m.layoutDimensions()
		m.syncHeaderInputWidths()
		m.ensureCursorVisible(m.listHeight(lay.innerH))
		m.syncPreviewViewport(false)
		m.syncAccountViewport(false)
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
		m.rebuildSearchIndex()
		m.refreshRows(true)
		m.syncPreviewViewport(true)
		return m, nil

	case usageLoadedMsg:
		m.accountRefreshing = false
		if msg.err != nil {
			m.accountErr = msg.err.Error()
		} else {
			m.accountErr = ""
		}
		if msg.found {
			m.accountUsage = msg.usage
			m.accountLoaded = true
		}
		m.accountVersion++
		m.syncAccountViewport(true)
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
		return m, tea.Batch(
			loadSessionsCmd(m.cfg),
			fetchAccountUsageCmd(m.cfg, true, m.now),
		)

	case tea.KeyMsg:
		if m.renaming {
			return m.updateRename(msg)
		}
		if m.confirmDelete != nil {
			return m.updateDeleteConfirm(msg)
		}
		if m.searching {
			return m.updateSearch(msg)
		}
		if m.showHelp {
			return m.updateHelp(msg)
		}
		if m.accountFocused {
			return m.updateAccountFocus(msg)
		}
		if m.previewFocused {
			return m.updatePreviewFocus(msg)
		}
		return m.updateBrowse(msg)
	}

	if m.renaming {
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return m, cmd
	}

	if m.searching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.renaming = false
		m.renameTargetID = ""
		m.renameInput.Blur()
		m.renameInput.SetValue("")
		return m, nil
	case tea.KeyEnter:
		m.commitRename()
		m.renaming = false
		m.renameTargetID = ""
		m.renameInput.Blur()
		m.renameInput.SetValue("")
		return m, nil
	}

	var cmd tea.Cmd
	m.renameInput, cmd = m.renameInput.Update(msg)
	return m, cmd
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
		m.syncHeaderInputWidths()
		cmd := m.searchInput.Focus()
		return m, cmd
	case key.Matches(msg, m.keys.Clear):
		if m.searchInput.Value() != "" {
			m.searchInput.SetValue("")
			m.refreshRows(false)
		}
	case key.Matches(msg, m.keys.TogglePreviewFocus):
		m.previewFocused = true
		m.accountFocused = false
		m.syncPreviewViewport(false)
	case key.Matches(msg, m.keys.ToggleAccount):
		return m.focusAccountPanel()
	case key.Matches(msg, m.keys.RefreshUsage):
		return m.startRefreshAccountUsage()
	case key.Matches(msg, m.keys.Rename):
		return m.startRename()
	case key.Matches(msg, m.keys.TogglePin):
		m.togglePin()
	case key.Matches(msg, m.keys.ToggleArchive):
		m.toggleArchive()
	case key.Matches(msg, m.keys.ToggleShowArchived):
		m.toggleShowArchived()
	case key.Matches(msg, m.keys.DeleteTrash):
		return m.startDeleteConfirm(session.DeleteModeTrash)
	case key.Matches(msg, m.keys.DeletePermanent):
		return m.startDeleteConfirm(session.DeleteModePermanent)
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
	case key.Matches(msg, m.keys.ToggleAccount):
		return m.focusAccountPanel()
	case key.Matches(msg, m.keys.RefreshUsage):
		return m.startRefreshAccountUsage()
	case key.Matches(msg, m.keys.Search):
		m.previewFocused = false
		m.accountFocused = false
		m.searching = true
		m.errMsg = ""
		m.syncHeaderInputWidths()
		cmd := m.searchInput.Focus()
		return m, cmd
	case key.Matches(msg, m.keys.Rename):
		return m.startRename()
	case key.Matches(msg, m.keys.TogglePin):
		m.togglePin()
	case key.Matches(msg, m.keys.ToggleArchive):
		m.toggleArchive()
	case key.Matches(msg, m.keys.ToggleShowArchived):
		m.toggleShowArchived()
	case key.Matches(msg, m.keys.DeleteTrash):
		return m.startDeleteConfirm(session.DeleteModeTrash)
	case key.Matches(msg, m.keys.DeletePermanent):
		return m.startDeleteConfirm(session.DeleteModePermanent)
	case key.Matches(msg, m.keys.Confirm):
		return m.resumeSelected()
	default:
		scrollViewport(&m.preview, msg, m.keys)
	}
	return m, nil
}

func (m Model) updateAccountFocus(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = true
		m.helpOffset = 0
		return m, nil
	case key.Matches(msg, m.keys.ToggleAccount), key.Matches(msg, m.keys.TogglePreviewFocus), msg.Type == tea.KeyEsc:
		m.accountFocused = false
		return m, nil
	case key.Matches(msg, m.keys.RefreshUsage):
		return m.startRefreshAccountUsage()
	case key.Matches(msg, m.keys.Search):
		m.accountFocused = false
		m.searching = true
		m.errMsg = ""
		m.syncHeaderInputWidths()
		cmd := m.searchInput.Focus()
		return m, cmd
	default:
		scrollViewport(&m.account, msg, m.keys)
	}
	return m, nil
}

func scrollViewport(vp *viewport.Model, msg tea.KeyMsg, keys KeyMap) bool {
	switch {
	case key.Matches(msg, keys.Up):
		vp.LineUp(1)
	case key.Matches(msg, keys.Down):
		vp.LineDown(1)
	case key.Matches(msg, keys.PageUp):
		vp.HalfPageUp()
	case key.Matches(msg, keys.PageDown):
		vp.HalfPageDown()
	case key.Matches(msg, keys.Top):
		vp.GotoTop()
	case key.Matches(msg, keys.Bottom):
		vp.GotoBottom()
	default:
		return false
	}
	return true
}

func (m Model) focusAccountPanel() (tea.Model, tea.Cmd) {
	m.previewFocused = false
	m.accountFocused = true
	m.syncAccountViewport(true)
	if !m.accountRefreshing && (!m.accountLoaded || m.accountUsage.Stale || len(m.accountUsage.Groups) == 0) {
		return m.startRefreshAccountUsage()
	}
	return m, nil
}

func (m Model) startRefreshAccountUsage() (tea.Model, tea.Cmd) {
	m.accountRefreshing = true
	m.accountErr = ""
	m.accountVersion++
	m.syncAccountViewport(true)
	return m, fetchAccountUsageCmd(m.cfg, true, m.now)
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

func (m Model) startDeleteConfirm(mode session.DeleteMode) (tea.Model, tea.Cmd) {
	sel, ok := m.selectedSession()
	if !ok {
		return m, nil
	}

	plan, ok := session.CollectDeletePlan(m.sessions, sel.ID)
	if !ok {
		plan = session.DeletePlan{Target: sel}
	}

	parentTitle := ""
	if sel.ParentID != "" {
		if parent, hasParent := m.sessions[strings.ToLower(strings.TrimSpace(sel.ParentID))]; hasParent {
			parentTitle = parent.Title
		}
	}

	trashPath := ""
	if mode == session.DeleteModeTrash {
		if td, err := session.ResolveTrashDir(); err == nil {
			trashPath = utils.ShortenHome(td)
		}
	}

	m.showHelp = false
	m.previewFocused = false
	m.accountFocused = false
	m.errMsg = ""
	m.confirmDelete = &deleteConfirmState{
		mode:           mode,
		plan:           plan,
		parentTitle:    parentTitle,
		trashPath:      trashPath,
		confirmFocused: mode == session.DeleteModeTrash,
	}
	return m, nil
}

func (m Model) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.confirmDelete = nil
		return m, nil
	case tea.KeyEnter:
		if m.confirmDelete != nil && m.confirmDelete.confirmFocused {
			m.executePendingDelete()
		} else {
			m.confirmDelete = nil
		}
		return m, nil
	case tea.KeyTab, tea.KeyShiftTab, tea.KeyLeft, tea.KeyRight:
		if m.confirmDelete != nil {
			m.confirmDelete.confirmFocused = !m.confirmDelete.confirmFocused
		}
		return m, nil
	}

	switch msg.String() {
	case "y", "Y":
		m.executePendingDelete()
		return m, nil
	case "n", "N", "q":
		m.confirmDelete = nil
		return m, nil
	case "h", "l":
		if m.confirmDelete != nil {
			m.confirmDelete.confirmFocused = !m.confirmDelete.confirmFocused
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) executePendingDelete() {
	pending := m.confirmDelete
	m.confirmDelete = nil
	if pending == nil {
		return
	}

	res, err := session.DeleteSession(m.cfg, m.sessions, pending.plan.Target.ID, pending.mode)
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	m.errMsg = ""

	deletedSet := make(map[string]bool, len(res.DeletedIDs))
	for _, id := range res.DeletedIDs {
		deletedSet[id] = true
		delete(m.sessions, id)
		delete(m.artifactCache, id)
		delete(m.expanded, id)
	}

	if parentID := strings.ToLower(strings.TrimSpace(pending.plan.Target.ParentID)); parentID != "" {
		if parent, ok := m.sessions[parentID]; ok && len(parent.SubSessions) > 0 {
			filtered := make([]session.Session, 0, len(parent.SubSessions))
			for _, sub := range parent.SubSessions {
				if !deletedSet[strings.ToLower(strings.TrimSpace(sub.ID))] {
					filtered = append(filtered, sub)
				}
			}
			parent.SubSessions = filtered
			m.sessions[parentID] = parent
			if len(filtered) == 0 {
				delete(m.expanded, parentID)
			}
		}
	}

	if m.userState.Delete(res.DeletedIDs...) {
		if err := session.SaveUserState(m.cfg.StateFile, m.userState); err != nil {
			m.errMsg = err.Error()
		}
	}

	m.rebuildSearchIndex()
	m.refreshRows(true)
	m.syncPreviewViewport(true)
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
	cmd, err := session.NewResumeCmdWithDir(selected.ID, selected.WorkDir)
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	if m.exitOnResume {
		m.pendingResume = &ResumeTarget{
			ID:      selected.ID,
			WorkDir: selected.WorkDir,
		}
		return m, tea.Quit
	}
	return m, tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return sessionResumedMsg{err: execErr}
	})
}

func (m Model) startRename() (tea.Model, tea.Cmd) {
	sel, ok := m.selectedSession()
	if !ok {
		return m, nil
	}
	m.renaming = true
	m.searching = false
	m.previewFocused = false
	m.accountFocused = false
	m.showHelp = false
	m.errMsg = ""
	m.renameTargetID = sel.ID

	initial := sel.Title
	if sel.TitleSource == session.TitleSourceNone {
		initial = ""
	}
	m.renameInput.SetValue(initial)
	m.renameInput.CursorEnd()
	m.syncHeaderInputWidths()
	return m, m.renameInput.Focus()
}

func (m *Model) commitRename() {
	if m.renameTargetID == "" {
		return
	}
	s, exists := m.sessions[m.renameTargetID]
	if !exists {
		return
	}

	clean := session.NormalizeTitle(m.renameInput.Value())
	m.userState.SetTitle(m.renameTargetID, clean)
	if err := session.SaveUserState(m.cfg.StateFile, m.userState); err != nil {
		m.errMsg = err.Error()
	} else {
		m.errMsg = ""
	}

	if s.DefaultTitle == "" && s.TitleSource != session.TitleSourceCustom && s.Title != "" {
		s.DefaultTitle = s.Title
		s.DefaultTitleSource = s.TitleSource
	}

	if clean != "" {
		s.Title = clean
		s.TitleSource = session.TitleSourceCustom
	} else {
		session.RestoreDefaultTitle(&s)
	}
	m.sessions[m.renameTargetID] = s

	// Keep parent SubSessions cache synchronized when renaming a child subsession.
	if s.ParentID != "" {
		if parent, hasParent := m.sessions[s.ParentID]; hasParent {
			for i, sub := range parent.SubSessions {
				if sub.ID == s.ID {
					parent.SubSessions[i] = s
					break
				}
			}
			m.sessions[s.ParentID] = parent
		}
	}

	m.rebuildSearchIndex()
	m.refreshRows(true)
	m.syncPreviewViewport(true)
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
	m.rebuildSearchIndex()
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
	m.rebuildSearchIndex()
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

func (m *Model) rebuildSearchIndex() {
	m.searchIndex = session.NewSearchIndex(m.sessions)
}

func (m *Model) refreshRows(preserveSelection bool) {
	selectedID := ""
	fallbackCursor := 0
	if preserveSelection && m.cursor >= 0 && m.cursor < len(m.rows) {
		selectedID = m.rows[m.cursor].Session.ID
		fallbackCursor = m.cursor
	}
	opts := session.SearchOptions{IncludeArchived: m.showArchived}
	if m.searchIndex != nil {
		m.results = m.searchIndex.Search(m.searchInput.Value(), opts)
	} else {
		m.results = session.SearchWithOptions(m.sessions, m.searchInput.Value(), opts)
	}
	m.syncHeaderInputWidths()
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
	m.renameInput.PromptStyle = theme.SearchPrompt
	m.renameInput.PlaceholderStyle = theme.HeaderHint
	m.syncPreviewViewport(true)
	m.syncAccountViewport(true)
}

func (m *Model) syncAccountViewport(force bool) {
	lay := m.layoutDimensions()
	bodyH := m.listHeight(lay.innerH)
	accountW := lay.accountInnerW

	m.account.Width = accountW
	m.account.Height = bodyH

	if !force && m.accountWidth == accountW && m.accountVersion > 0 {
		return
	}

	content := components.FormatAccountUsage(
		m.accountUsage,
		!m.accountLoaded && m.accountRefreshing,
		m.accountRefreshing,
		m.accountErr,
		accountW,
		m.now(),
		m.theme,
	)
	m.account.SetContent(content)
	if m.accountVersion == 0 {
		m.account.GotoTop()
	}
	m.accountWidth = accountW
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
	gap := strings.Repeat(" ", paneGapCols)

	var modal string
	switch {
	case lay.threePane:
		modal = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderPreviewBox(lay), gap,
			m.renderListBox(lay), gap,
			m.renderAccountBox(lay),
		)
	case lay.twoPane && m.accountFocused:
		modal = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderListBox(lay), gap,
			m.renderAccountBox(lay),
		)
	case lay.twoPane:
		modal = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderPreviewBox(lay), gap,
			m.renderListBox(lay),
		)
	case m.accountFocused:
		modal = m.renderAccountBox(lay)
	case m.previewFocused:
		modal = m.renderPreviewBox(lay)
	default:
		modal = m.renderListBox(lay)
	}

	base := modal
	if m.width > 0 && m.height > 0 {
		base = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}

	if m.confirmDelete != nil {
		dialog := components.RenderConfirmDialog(components.ConfirmDialog{
			Permanent:      m.confirmDelete.mode == session.DeleteModePermanent,
			Title:          m.confirmDelete.plan.Target.Title,
			SessionID:      m.confirmDelete.plan.Target.ID,
			ParentTitle:    m.confirmDelete.parentTitle,
			SubCount:       len(m.confirmDelete.plan.Children),
			TrashPath:      m.confirmDelete.trashPath,
			ConfirmFocused: m.confirmDelete.confirmFocused,
		}, m.width, m.height, m.theme)
		return components.OverlayCenter(base, dialog, m.width, m.height)
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
	if !m.showHelp && m.confirmDelete == nil && (m.searching || m.renaming || (!m.previewFocused && !m.accountFocused)) {
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
	title := "Preview"
	if m.previewFocused {
		title = "Preview [scroll]"
	}
	focused := !m.showHelp && m.confirmDelete == nil && m.previewFocused && !m.searching && !m.renaming
	return m.renderViewportBox(
		title,
		focused,
		m.previewFooterView(lay.previewInnerW),
		lay.previewBoxW,
		lay.previewInnerW,
		lay,
		m.preview,
		m.loading,
	)
}

func (m Model) renderAccountBox(lay layoutSpec) string {
	title := "Account"
	if tier := strings.TrimSpace(m.accountUsage.PlanTier); tier != "" {
		title = "Account · " + tier
	}
	if m.accountFocused {
		title += " [scroll]"
	} else if m.accountRefreshing {
		title += " [sync]"
	}
	focused := !m.showHelp && m.confirmDelete == nil && m.accountFocused && !m.searching && !m.renaming
	return m.renderViewportBox(
		title,
		focused,
		m.accountFooterView(lay.accountInnerW),
		lay.accountBoxW,
		lay.accountInnerW,
		lay,
		m.account,
		!m.accountLoaded && m.accountRefreshing,
	)
}

func (m Model) renderViewportBox(title string, focused bool, footer string, boxW, innerW int, lay layoutSpec, vp viewport.Model, hideEmptyScroll bool) string {
	innerH := lay.innerH

	lines := make([]string, 0, innerH)
	lines = append(lines, m.viewportHeaderView(title, focused, innerW, vp, hideEmptyScroll))
	lines = append(lines, m.theme.Divider.Render(strings.Repeat("─", innerW)))

	vp.Width = innerW
	vp.Height = m.listHeight(innerH)
	lines = append(lines, vp.View())
	lines = append(lines, footer)

	content := strings.Join(lines, "\n")

	borderStyle := m.theme.ModalBorder
	if focused {
		borderStyle = m.theme.ModalBorderActive
	}

	return borderStyle.
		Width(innerW + 4).
		Height(innerH + 2).
		MaxWidth(boxW).
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

func (m Model) headerLayout(innerW int, prompt string) (counter string, maxLeft int, inputWidth int) {
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

	promptW := lipgloss.Width(prompt)
	// Reserve Prompt and 1 cursor cell added by textinput.View().
	inputWidth = max(1, maxLeft-promptW-1)
	return counter, maxLeft, inputWidth
}

func (m *Model) syncHeaderInputWidths() {
	_, _, innerW, _ := m.boxDimensions()
	_, _, searchW := m.headerLayout(innerW, m.searchInput.Prompt)
	m.searchInput.Width = searchW
	_, _, renameW := m.headerLayout(innerW, m.renameInput.Prompt)
	m.renameInput.Width = renameW
}

func (m Model) renderHeaderInput(input textinput.Model, width, maxLeft int) string {
	input.Width = width
	rendered := input.View()
	if lipgloss.Width(rendered) > maxLeft {
		return lipgloss.NewStyle().MaxWidth(maxLeft).Render(rendered)
	}
	return rendered
}

func (m Model) headerView(innerW int) string {
	activePrompt := m.searchInput.Prompt
	if m.renaming {
		activePrompt = m.renameInput.Prompt
	}
	counter, maxLeft, inputW := m.headerLayout(innerW, activePrompt)

	var leftStyled string
	switch {
	case m.renaming:
		leftStyled = m.renderHeaderInput(m.renameInput, inputW, maxLeft)
	case m.searching:
		leftStyled = m.renderHeaderInput(m.searchInput, inputW, maxLeft)
	case m.searchInput.Value() != "":
		leftStyled = m.theme.SearchPrompt.Render(components.TruncateWidth("/ "+m.searchInput.Value(), maxLeft))
	default:
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
	if m.previewFocused {
		title = "Preview [scroll]"
	}
	return m.viewportHeaderView(title, m.previewFocused, innerW, m.preview, m.loading)
}

func (m Model) viewportHeaderView(title string, focused bool, innerW int, vp viewport.Model, hideEmptyScroll bool) string {
	titleStyle := m.theme.HeaderHint
	if focused {
		titleStyle = m.theme.SearchPrompt
	}

	pctText := ""
	if vp.TotalLineCount() > vp.Height && vp.Height > 0 {
		pct := int(math.Round(vp.ScrollPercent() * 100))
		pctText = fmt.Sprintf("%d%%", pct)
	} else if vp.TotalLineCount() > 0 && !hideEmptyScroll {
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
	switch {
	case m.renaming:
		hint = "Enter save · Esc cancel · empty resets"
	case m.searching:
		hint = "↑↓ select · Enter open · Esc clear"
	case m.searchInput.Value() != "":
		hint = "↑↓ select · Enter open · Esc clear · ? keys · q quit"
	default:
		hint = "↑↓ select · / search · u account · ? keys"
	}
	return m.theme.Footer.Render(components.TruncateWidth(hint, innerW))
}

func (m Model) previewFooterView(innerW int) string {
	var hint string
	if m.previewFocused {
		hint = "↑↓/Pg scroll · u account · Tab back · ? keys"
	} else {
		hint = "Tab focus preview · u account · ? keys"
	}
	return m.theme.Footer.Render(components.TruncateWidth(hint, innerW))
}

func (m Model) accountFooterView(innerW int) string {
	var hint string
	if m.accountFocused {
		hint = "↑↓/Pg scroll · U refresh · u/Esc back · ? keys"
	} else {
		hint = "u focus account · U refresh · ? keys"
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
		{Desc: "Toggle / focus account usage", Key: "u"},
		{Desc: "Refresh account quota limits", Key: "U"},
		{Desc: "Rename selected session", Key: "r"},
		{Desc: "Pin / unpin session", Key: "p"},
		{Desc: "Archive / unarchive session", Key: "a"},
		{Desc: "Show / hide archived sessions", Key: "A"},
		{Desc: "Move session to trash", Key: "d"},
		{Desc: "Delete session permanently", Key: "D"},
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
			threePane:     false,
			boxH:          boxH,
			innerH:        innerH,
			listBoxW:      boxW,
			listInnerW:    innerW,
			previewBoxW:   boxW,
			previewInnerW: innerW,
			accountBoxW:   boxW,
			accountInnerW: innerW,
		}
	}

	if termW >= threePaneMinWidth {
		totalMinW := min(max(36, termW-2), 128)
		totalMaxW := min(max(36, termW-2), 212)
		totalW := clampInt(termW*94/100, totalMinW, totalMaxW)

		availW := max(30, totalW-2*paneGapCols)
		listBoxW := availW * 34 / 100
		if listBoxW < 40 {
			listBoxW = 40
		}
		previewBoxW := availW * 31 / 100
		if previewBoxW < 34 {
			previewBoxW = 34
		}
		accountBoxW := availW - listBoxW - previewBoxW
		if accountBoxW < 34 {
			accountBoxW = 34
			rem := max(20, availW-accountBoxW)
			listBoxW = rem * 52 / 100
			previewBoxW = rem - listBoxW
		}

		return layoutSpec{
			twoPane:       true,
			threePane:     true,
			boxH:          boxH,
			innerH:        innerH,
			listBoxW:      listBoxW,
			listInnerW:    max(10, listBoxW-6),
			previewBoxW:   previewBoxW,
			previewInnerW: max(10, previewBoxW-6),
			accountBoxW:   accountBoxW,
			accountInnerW: max(10, accountBoxW-6),
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
		threePane:     false,
		boxH:          boxH,
		innerH:        innerH,
		listBoxW:      listBoxW,
		listInnerW:    max(10, listBoxW-6),
		previewBoxW:   previewBoxW,
		previewInnerW: max(10, previewBoxW-6),
		accountBoxW:   previewBoxW,
		accountInnerW: max(10, previewBoxW-6),
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

func fetchAccountUsageCmd(cfg config.Config, force bool, nowFn func() time.Time) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		if nowFn != nil {
			now = nowFn()
		}
		if !force {
			if cached, ok, needsRefresh := session.LoadCachedAccountUsage(cfg, now); ok && !needsRefresh {
				return usageLoadedMsg{usage: cached, found: true}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), usageRefreshTimeout)
		defer cancel()

		live, err := session.FetchLiveAccountUsage(ctx, cfg, now)
		if err == nil {
			return usageLoadedMsg{usage: live, found: true}
		}

		if cached, ok, _ := session.LoadCachedAccountUsage(cfg, now); ok {
			return usageLoadedMsg{usage: cached, found: true, err: err}
		}
		return usageLoadedMsg{err: err}
	}
}
