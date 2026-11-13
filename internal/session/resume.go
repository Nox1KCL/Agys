package session

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Nox1KCL/Agys/internal/utils"
)

const (
	agyBinaryName          = "agy"
	maxResumeAncestorWalks = 4
)

// ResolveBinary locates the official agy CLI executable in PATH or common Linux user/system bin directories.
func ResolveBinary() (string, error) {
	if path, err := exec.LookPath(agyBinaryName); err == nil {
		if abs, absErr := filepath.Abs(path); absErr == nil {
			return abs, nil
		}
		return path, nil
	}

	var candidates []string
	if xdgBin := strings.TrimSpace(os.Getenv("XDG_BIN_HOME")); xdgBin != "" {
		if resolvedXDG, err := utils.ResolveAbsPath(filepath.Join(xdgBin, agyBinaryName)); err == nil && resolvedXDG != "" {
			candidates = append(candidates, resolvedXDG)
		}
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", agyBinaryName),
			filepath.Join(home, "bin", agyBinaryName),
		)
	}

	candidates = append(candidates,
		filepath.Join("/usr/local/bin", agyBinaryName),
		filepath.Join("/usr/bin", agyBinaryName),
	)

	seen := make(map[string]struct{}, len(candidates))
	for _, cand := range candidates {
		clean := filepath.Clean(cand)
		if _, dup := seen[clean]; dup {
			continue
		}
		seen[clean] = struct{}{}
		if isExecutableFile(clean) {
			return clean, nil
		}
	}

	return "", fmt.Errorf("%s binary not found in PATH or standard bin directories", agyBinaryName)
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// ResolveResumeDir resolves workDir to an existing non-root directory suitable for cmd.Dir.
// If the exact directory no longer exists on disk, up to maxResumeAncestorWalks existing
// parent directories are checked (never returning "/") before falling back to "".
func ResolveResumeDir(workDir string) string {
	abs, err := utils.ResolveAbsPath(workDir)
	if err != nil || abs == "" || abs == "/" {
		return ""
	}
	if utils.DirExists(abs) {
		return abs
	}

	home, _ := os.UserHomeDir()
	underHome := home != "" && strings.HasPrefix(abs, filepath.Clean(home)+string(filepath.Separator))

	cur := filepath.Dir(abs)
	for depth := 0; depth < maxResumeAncestorWalks; depth++ {
		if cur == "" || cur == "/" || cur == "." {
			break
		}
		if utils.DirExists(cur) {
			return cur
		}
		if underHome && cur == filepath.Clean(home) {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	return ""
}

// NewResumeCmd builds an interactive command to resume the given Antigravity session UUID.
func NewResumeCmd(sessionID string) (*exec.Cmd, error) {
	return NewResumeCmdWithDir(sessionID, "")
}

// NewResumeCmdWithDir builds an interactive command to resume sessionID in workDir (when available).
func NewResumeCmdWithDir(sessionID, workDir string) (*exec.Cmd, error) {
	cleanID := strings.ToLower(strings.TrimSpace(sessionID))
	if !utils.IsUUIDv4(cleanID) {
		return nil, fmt.Errorf("invalid session UUID v4: %q", sessionID)
	}

	bin, err := ResolveBinary()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, "--conversation="+cleanID)
	if dir := ResolveResumeDir(workDir); dir != "" {
		cmd.Dir = dir
	}
	return cmd, nil
}

// Resume launches the official agy CLI attached to the current terminal for sessionID.
func Resume(sessionID string) error {
	return ResumeWithDir(sessionID, "")
}

// ResumeWithDir launches agy attached to the current terminal in workDir (when available).
func ResumeWithDir(sessionID, workDir string) error {
	cmd, err := NewResumeCmdWithDir(sessionID, workDir)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Keep the parent process alive on Ctrl+C while the interactive child handles SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resume session %s: %w", sessionID, err)
	}

	return nil
}

// ExecResume replaces the current process with agy for sessionID in workDir (when available).
func ExecResume(sessionID, workDir string) error {
	cmd, err := NewResumeCmdWithDir(sessionID, workDir)
	if err != nil {
		return err
	}
	if cmd.Dir != "" {
		if chdirErr := os.Chdir(cmd.Dir); chdirErr != nil {
			return fmt.Errorf("change directory to %q: %w", cmd.Dir, chdirErr)
		}
	}
	if execErr := syscall.Exec(cmd.Path, cmd.Args, os.Environ()); execErr != nil {
		return fmt.Errorf("exec %s: %w", cmd.Path, execErr)
	}
	return nil
}
