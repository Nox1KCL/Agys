package session

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/Nox1KCL/Agys/internal/utils"
)

const agyBinaryName = "agy"

// ResolveBinary locates the official agy CLI executable in PATH or ~/.local/bin/agy.
func ResolveBinary() (string, error) {
	if path, err := exec.LookPath(agyBinaryName); err == nil {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate %s binary: %w", agyBinaryName, err)
	}

	fallback := filepath.Join(home, ".local", "bin", agyBinaryName)
	info, statErr := os.Stat(fallback)
	if statErr != nil || info.IsDir() {
		return "", fmt.Errorf("%s binary not found in PATH or %s", agyBinaryName, fallback)
	}

	return fallback, nil
}

// NewResumeCmd builds an interactive command to resume the given Antigravity session UUID.
func NewResumeCmd(sessionID string) (*exec.Cmd, error) {
	cleanID := strings.ToLower(strings.TrimSpace(sessionID))
	if !utils.IsUUIDv4(cleanID) {
		return nil, fmt.Errorf("invalid session UUID v4: %q", sessionID)
	}

	bin, err := ResolveBinary()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, "--conversation="+cleanID)
	return cmd, nil
}

// Resume launches the official agy CLI attached to the current terminal for sessionID.
func Resume(sessionID string) error {
	cmd, err := NewResumeCmd(sessionID)
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
