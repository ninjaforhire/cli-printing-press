package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// StaleLockThreshold is the duration after which a lock is considered stale.
	StaleLockThreshold = 30 * time.Minute

	locksDir = ".locks"
)

// LockState represents the state of a build lock for a CLI.
type LockState struct {
	Scope      string    `json:"scope"`
	Phase      string    `json:"phase"`
	PID        int       `json:"pid"`
	AcquiredAt time.Time `json:"acquired_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// LockStatusResult is the combined status returned by LockStatus.
type LockStatusResult struct {
	Held       bool       `json:"held"`
	Stale      bool       `json:"stale"`
	Phase      string     `json:"phase,omitempty"`
	Scope      string     `json:"scope,omitempty"`
	AgeSeconds float64    `json:"age_seconds,omitempty"`
	HasCLI     bool       `json:"has_cli"`
	Lock       *LockState `json:"lock,omitempty"`
}

// LocksDir returns the global locks directory path.
func LocksDir() string {
	return filepath.Join(PressHome(), locksDir)
}

// LockFilePath returns the lock file path for a given CLI name.
func LockFilePath(cliName string) string {
	return filepath.Join(LocksDir(), cliName+".lock")
}

// AcquireLock attempts to acquire a build lock for the given CLI.
// It auto-reclaims stale locks. If force is true, it overrides even fresh
// locks held by a different scope.
func AcquireLock(cliName, scope string, force bool) (*LockState, error) {
	lockPath := LockFilePath(cliName)

	if err := os.MkdirAll(LocksDir(), 0o755); err != nil {
		return nil, fmt.Errorf("creating locks directory: %w", err)
	}

	lock := &LockState{
		Scope:      scope,
		Phase:      "acquire",
		PID:        os.Getpid(),
		AcquiredAt: time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Try atomic creation first.
	err := writeLockExclusive(lockPath, lock)
	if err == nil {
		return lock, nil
	}
	if !os.IsExist(err) {
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}

	// Lock file exists — check if we can reclaim it.
	existing, readErr := readLock(lockPath)
	if readErr != nil {
		// Can't read existing lock — try to remove and re-create.
		_ = os.Remove(lockPath)
		if err := writeLockExclusive(lockPath, lock); err != nil {
			return nil, fmt.Errorf("acquiring lock after removing unreadable lock: %w", err)
		}
		return lock, nil
	}

	// Same scope — re-entrant, just overwrite.
	if existing.Scope == scope {
		if err := writeLock(lockPath, lock); err != nil {
			return nil, fmt.Errorf("re-acquiring lock for same scope: %w", err)
		}
		return lock, nil
	}

	// Different scope — check staleness or force.
	if IsStale(existing) || force {
		_ = os.Remove(lockPath)
		if err := writeLockExclusive(lockPath, lock); err != nil {
			return nil, fmt.Errorf("acquiring lock after reclaim: %w", err)
		}
		return lock, nil
	}

	return nil, fmt.Errorf("lock held by scope %q (phase: %s, updated: %s ago)", existing.Scope, existing.Phase, time.Since(existing.UpdatedAt).Truncate(time.Second))
}

// UpdateLock refreshes the heartbeat and phase of an existing lock.
func UpdateLock(cliName, phase string) error {
	lockPath := LockFilePath(cliName)

	existing, err := readLock(lockPath)
	if err != nil {
		return fmt.Errorf("reading lock for update: %w", err)
	}

	existing.Phase = phase
	existing.UpdatedAt = time.Now()
	existing.PID = os.Getpid()

	return writeLock(lockPath, existing)
}

// LockStatus returns the current lock state for a CLI, including whether
// a completed CLI exists in the library.
func LockStatus(cliName string) LockStatusResult {
	result := LockStatusResult{}

	// Check library for completed CLI.
	libDir := filepath.Join(PublishedLibraryRoot(), cliName)
	if info, err := os.Stat(libDir); err == nil && info.IsDir() {
		goModPath := filepath.Join(libDir, "go.mod")
		manifestPath := filepath.Join(libDir, CLIManifestFilename)
		_, goModErr := os.Stat(goModPath)
		_, manifestErr := os.Stat(manifestPath)
		result.HasCLI = goModErr == nil || manifestErr == nil
	}

	// Check lock file.
	lockPath := LockFilePath(cliName)
	lock, err := readLock(lockPath)
	if err != nil {
		return result
	}

	result.Held = true
	result.Stale = IsStale(lock)
	result.Phase = lock.Phase
	result.Scope = lock.Scope
	result.AgeSeconds = time.Since(lock.UpdatedAt).Seconds()
	result.Lock = lock

	return result
}

// ReleaseLock removes the lock file for a CLI. It is idempotent.
func ReleaseLock(cliName string) error {
	lockPath := LockFilePath(cliName)
	err := os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("releasing lock: %w", err)
	}
	return nil
}

// PromoteWorkingCLI copies a working CLI directory to the library, writes
// the CLI manifest, updates the CurrentRunPointer, and releases the lock.
func PromoteWorkingCLI(cliName, workingDir string, state *PipelineState) error {
	if workingDir == "" {
		return fmt.Errorf("working directory is empty")
	}

	// Verify working dir has content.
	entries, err := os.ReadDir(workingDir)
	if err != nil {
		return fmt.Errorf("reading working directory: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("working directory is empty: %s", workingDir)
	}

	libraryDir := filepath.Join(PublishedLibraryRoot(), cliName)

	// Clear existing library contents if present.
	if _, err := os.Stat(libraryDir); err == nil {
		if err := os.RemoveAll(libraryDir); err != nil {
			return fmt.Errorf("clearing existing library directory: %w", err)
		}
	}

	// Ensure parent exists.
	if err := os.MkdirAll(filepath.Dir(libraryDir), 0o755); err != nil {
		return fmt.Errorf("creating library parent directory: %w", err)
	}

	// Copy working dir to library.
	if err := CopyDir(workingDir, libraryDir); err != nil {
		return fmt.Errorf("copying to library: %w", err)
	}

	// Update state to reflect promotion.
	state.PublishedDir = libraryDir

	// Write CLI manifest.
	if err := writeCLIManifestForPublish(state, libraryDir); err != nil {
		return fmt.Errorf("writing CLI manifest: %w", err)
	}

	// Update current run pointer so working_dir reflects library path.
	state.WorkingDir = libraryDir
	if err := state.Save(); err != nil {
		return fmt.Errorf("updating state after promotion: %w", err)
	}

	// Release the lock.
	if err := ReleaseLock(cliName); err != nil {
		return fmt.Errorf("releasing lock after promotion: %w", err)
	}

	return nil
}

// IsStale returns true if the lock's UpdatedAt is older than StaleLockThreshold.
func IsStale(lock *LockState) bool {
	return time.Since(lock.UpdatedAt) > StaleLockThreshold
}

func readLock(path string) (*LockState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock LockState
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	return &lock, nil
}

func writeLock(path string, lock *LockState) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeLockExclusive(path string, lock *LockState) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
