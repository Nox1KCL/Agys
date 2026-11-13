package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/utils"
)

// PluginUsageSummary embeds AccountUsage along with the minimum remaining percentage across buckets.
type PluginUsageSummary struct {
	AccountUsage
	MinRemainingPercent int `json:"min_remaining_percent"`
}

// PluginSessionItem is a compact serializable representation of a session for Omarchy bar widgets.
type PluginSessionItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	WorkDir     string `json:"work_dir,omitempty"`
	DisplayDir  string `json:"display_dir,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	UpdatedUnix int64  `json:"updated_unix"`
	Pinned      bool   `json:"pinned"`
	Archived    bool   `json:"archived"`
	Active      bool   `json:"active"`
	Working     bool   `json:"working"`
	SubCount    int    `json:"sub_count,omitempty"`
}

// PluginTrashItem summarizes a session residing in FreeDesktop Trash.
type PluginTrashItem struct {
	TrashName   string `json:"trash_name"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	DeletedAt   string `json:"deleted_at,omitempty"`
	DeletedUnix int64  `json:"deleted_unix"`
}

// PluginSnapshot is the full JSON payload emitted by "agys --plugin-state".
type PluginSnapshot struct {
	Ready        bool                `json:"ready"`
	GeneratedAt  string              `json:"generated_at"`
	ActiveCount  int                 `json:"active_count"`
	ActiveStatus string              `json:"active_status"`
	ActiveIDs    []string            `json:"active_ids"`
	Usage        PluginUsageSummary  `json:"usage"`
	UsageError   string              `json:"usage_error,omitempty"`
	Pinned       []PluginSessionItem `json:"pinned"`
	Recent       []PluginSessionItem `json:"recent"`
	Archived     []PluginSessionItem `json:"archived"`
	Trash        []PluginTrashItem   `json:"trash"`
}

// PluginActionResult confirms completion of a headless session action triggered by the Omarchy plugin.
type PluginActionResult struct {
	OK         bool     `json:"ok"`
	Action     string   `json:"action"`
	ID         string   `json:"id"`
	Pinned     *bool    `json:"pinned,omitempty"`
	Archived   *bool    `json:"archived,omitempty"`
	DeletedIDs []string `json:"deleted_ids,omitempty"`
}

// BuildPluginSnapshot collects sessions, user overrides, live presence, quota usage, and Trash entries.
func BuildPluginSnapshot(ctx context.Context, cfg config.Config, refreshUsage bool, now time.Time) (PluginSnapshot, error) {
	if now.IsZero() {
		now = time.Now()
	}

	sessions, err := Collect(cfg)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return PluginSnapshot{}, err
		}
		sessions = make(map[string]Session)
	}

	if st, stateErr := LoadUserState(cfg.StateFile); stateErr == nil {
		ApplyUserState(sessions, st)
	}

	presence := DetectActiveSessions(cfg)
	activeIDs := make([]string, 0, len(presence))
	anyWorking := false
	for id, p := range presence {
		activeIDs = append(activeIDs, id)
		if p.Working {
			anyWorking = true
		}
	}
	sort.Strings(activeIDs)

	activeStatus := "Idle"
	if len(activeIDs) > 0 {
		if anyWorking {
			activeStatus = "Working"
		} else {
			activeStatus = "Waiting"
		}
	}

	roots := SortedRoots(sessions)
	pinned := make([]PluginSessionItem, 0)
	recent := make([]PluginSessionItem, 0)
	archived := make([]PluginSessionItem, 0)

	for _, s := range roots {
		item := toPluginSessionItem(s, presence)
		switch {
		case s.Archived:
			archived = append(archived, item)
		case s.Pinned:
			pinned = append(pinned, item)
		default:
			recent = append(recent, item)
		}
	}

	usage, usageErr := resolvePluginUsage(ctx, cfg, refreshUsage, now)
	usageSummary := PluginUsageSummary{
		AccountUsage:        usage,
		MinRemainingPercent: minRemainingPercent(usage),
	}

	trashedRaw, _ := ListTrashedSessions(cfg)
	trashItems := make([]PluginTrashItem, 0, len(trashedRaw))
	for _, t := range trashedRaw {
		deletedAt := ""
		var deletedUnix int64
		if !t.DeletedAt.IsZero() {
			deletedAt = t.DeletedAt.UTC().Format(time.RFC3339)
			deletedUnix = t.DeletedAt.Unix()
		}
		trashItems = append(trashItems, PluginTrashItem{
			TrashName:   t.TrashName,
			ID:          t.ID,
			Title:       t.Title,
			DeletedAt:   deletedAt,
			DeletedUnix: deletedUnix,
		})
	}

	return PluginSnapshot{
		Ready:        true,
		GeneratedAt:  now.UTC().Format(time.RFC3339),
		ActiveCount:  len(activeIDs),
		ActiveStatus: activeStatus,
		ActiveIDs:    activeIDs,
		Usage:        usageSummary,
		UsageError:   usageErr,
		Pinned:       pinned,
		Recent:       recent,
		Archived:     archived,
		Trash:        trashItems,
	}, nil
}

// RunPluginAction executes a single non-interactive state or filesystem mutation on targetID.
func RunPluginAction(cfg config.Config, action, targetID string) (PluginActionResult, error) {
	act := strings.ToLower(strings.TrimSpace(action))
	cleanTarget := strings.TrimSpace(targetID)
	if act == "" {
		return PluginActionResult{}, fmt.Errorf("missing action name")
	}
	if cleanTarget == "" {
		return PluginActionResult{}, fmt.Errorf("missing session id")
	}

	switch act {
	case "pin", "unpin", "toggle-pin", "archive", "unarchive", "toggle-archive":
		return runStateToggleAction(cfg, act, strings.ToLower(cleanTarget))
	case "trash":
		return runTrashAction(cfg, strings.ToLower(cleanTarget))
	case "restore":
		item, err := RestoreTrashedSession(cfg, cleanTarget)
		if err != nil {
			return PluginActionResult{}, err
		}
		return PluginActionResult{
			OK:     true,
			Action: act,
			ID:     item.ID,
		}, nil
	case "purge", "delete":
		if item, err := PurgeTrashedSession(cfg, cleanTarget); err == nil {
			return PluginActionResult{
				OK:         true,
				Action:     act,
				ID:         item.ID,
				DeletedIDs: []string{item.ID},
			}, nil
		}
		sessions, err := Collect(cfg)
		if err != nil {
			return PluginActionResult{}, err
		}
		res, err := DeleteSession(cfg, sessions, cleanTarget, DeleteModePermanent)
		if err != nil {
			return PluginActionResult{}, err
		}
		if st, stErr := LoadUserState(cfg.StateFile); stErr == nil && st.Delete(res.DeletedIDs...) {
			_ = SaveUserState(cfg.StateFile, st)
		}
		return PluginActionResult{
			OK:         true,
			Action:     act,
			ID:         strings.ToLower(cleanTarget),
			DeletedIDs: res.DeletedIDs,
		}, nil
	default:
		return PluginActionResult{}, fmt.Errorf("unsupported action %q", action)
	}
}

func runStateToggleAction(cfg config.Config, act, id string) (PluginActionResult, error) {
	if !utils.IsUUIDv4(id) {
		return PluginActionResult{}, fmt.Errorf("invalid session UUID %q", id)
	}
	st, err := LoadUserState(cfg.StateFile)
	if err != nil {
		return PluginActionResult{}, err
	}

	current := st.Sessions[id]
	curPinned := current.Pinned != nil && *current.Pinned
	curArchived := current.Archived != nil && *current.Archived

	res := PluginActionResult{
		OK:     true,
		Action: act,
		ID:     id,
	}

	switch act {
	case "pin":
		st.SetPinned(id, true)
		val := true
		res.Pinned = &val
	case "unpin":
		st.SetPinned(id, false)
		val := false
		res.Pinned = &val
	case "toggle-pin":
		next := !curPinned
		st.SetPinned(id, next)
		res.Pinned = &next
	case "archive":
		st.SetArchived(id, true)
		val := true
		res.Archived = &val
	case "unarchive":
		st.SetArchived(id, false)
		val := false
		res.Archived = &val
	case "toggle-archive":
		next := !curArchived
		st.SetArchived(id, next)
		res.Archived = &next
	}

	if err := SaveUserState(cfg.StateFile, st); err != nil {
		return PluginActionResult{}, err
	}
	return res, nil
}

func runTrashAction(cfg config.Config, id string) (PluginActionResult, error) {
	sessions, err := Collect(cfg)
	if err != nil {
		return PluginActionResult{}, err
	}
	res, err := DeleteSession(cfg, sessions, id, DeleteModeTrash)
	if err != nil {
		return PluginActionResult{}, err
	}
	if st, stErr := LoadUserState(cfg.StateFile); stErr == nil && st.Delete(res.DeletedIDs...) {
		_ = SaveUserState(cfg.StateFile, st)
	}
	return PluginActionResult{
		OK:         true,
		Action:     "trash",
		ID:         id,
		DeletedIDs: res.DeletedIDs,
	}, nil
}

func resolvePluginUsage(ctx context.Context, cfg config.Config, refreshUsage bool, now time.Time) (AccountUsage, string) {
	if refreshUsage {
		live, err := FetchLiveAccountUsage(ctx, cfg, now)
		if err == nil {
			if len(live.Groups) > 0 {
				return live, ""
			}
		}
		if cached, ok, _ := LoadCachedAccountUsage(cfg, now); ok {
			if err != nil {
				return cached, err.Error()
			}
			return cached, ""
		}
		if err != nil {
			return AccountUsage{}, err.Error()
		}
		return AccountUsage{}, ""
	}

	if cached, ok, _ := LoadCachedAccountUsage(cfg, now); ok {
		return cached, ""
	}
	return AccountUsage{}, ""
}

func toPluginSessionItem(s Session, presence map[string]PresenceState) PluginSessionItem {
	p := presence[strings.ToLower(s.ID)]
	active := p.Active
	working := p.Working
	for _, sub := range s.SubSessions {
		if sp, ok := presence[strings.ToLower(sub.ID)]; ok {
			if sp.Active {
				active = true
			}
			if sp.Working {
				working = true
			}
		}
	}

	updatedAt := ""
	var updatedUnix int64
	if !s.UpdatedAt.IsZero() {
		updatedAt = s.UpdatedAt.UTC().Format(time.RFC3339)
		updatedUnix = s.UpdatedAt.Unix()
	}

	title := strings.TrimSpace(s.Title)
	if title == "" {
		title = untitledSessionLabel
	}

	displayDir := ""
	if s.WorkDir != "" {
		displayDir = utils.ShortenHome(s.WorkDir)
	}

	return PluginSessionItem{
		ID:          s.ID,
		Title:       title,
		WorkDir:     s.WorkDir,
		DisplayDir:  displayDir,
		UpdatedAt:   updatedAt,
		UpdatedUnix: updatedUnix,
		Pinned:      s.Pinned,
		Archived:    s.Archived,
		Active:      active,
		Working:     working,
		SubCount:    len(s.SubSessions),
	}
}

func minRemainingPercent(usage AccountUsage) int {
	minPct := -1
	for _, g := range usage.Groups {
		for _, b := range g.Buckets {
			if minPct == -1 || b.RemainingPercent < minPct {
				minPct = b.RemainingPercent
			}
		}
	}
	return minPct
}
