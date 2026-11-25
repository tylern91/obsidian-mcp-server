package vault

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// atomicTempRe matches the decimal random tail createAtomicTemp appends to the
// ".BASE.tmp-" prefix writeFileAtomic uses.
var atomicTempRe = regexp.MustCompile(`^\..+\.tmp-[0-9]+$`)

// isAtomicTemp reports whether name is a writeFileAtomic temp file, so
// listings and walks never surface one that a concurrent write has in flight.
func isAtomicTemp(name string) bool {
	return atomicTempRe.MatchString(name)
}

// beforeRenameHook, when non-nil, runs after the temp file is written and
// synced and before it is renamed onto the target. A non-nil return aborts the
// write, which lets a test simulate a crash at the riskiest point.
var beforeRenameHook func() error

// maxTempBase bounds the target-name portion of a temp name so a long note
// name cannot push the temp name past the filesystem's 255-byte limit.
const maxTempBase = 100

// writeFileAtomic replaces the file at absPath with data so that a crash
// leaves either the old or the new content, never a truncated file.
//
// Callers must already have run checkSymlinksForWrite under s.mu. A symlinked
// note is written at its resolved target so the link itself survives, and the
// temp file is created in that target's directory, which is already inside the
// vault. The temp file is removed on every error path.
func (s *Service) writeFileAtomic(absPath string, data []byte) error {
	target, err := s.atomicTarget(absPath)
	if err != nil {
		return err
	}

	// An existing file keeps its exact mode. A new file is created with 0o644
	// through the umask, as os.WriteFile did, so a restrictive umask still holds.
	var existingMode os.FileMode
	replacing := false
	if info, statErr := os.Stat(target); statErr == nil {
		existingMode, replacing = info.Mode().Perm(), true
	}
	if replacing {
		// Rename needs only directory permission, so without this probe a
		// read-only note would be replaced where an in-place write fails.
		f, err := os.OpenFile(target, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		_ = f.Close()
	}

	dir := filepath.Dir(target)
	base := filepath.Base(target)
	if len(base) > maxTempBase {
		base = base[:maxTempBase]
	}
	tmp, err := createAtomicTemp(dir, "."+base+".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if replacing {
		if err := tmp.Chmod(existingMode); err != nil {
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if beforeRenameHook != nil {
		if err := beforeRenameHook(); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	committed = true
	s.gen.Add(1)

	// Best effort: the rename is already visible, so a failed directory sync
	// must not turn a completed write into an error.
	if d, openErr := os.Open(dir); openErr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// createAtomicTemp creates a new temp file in dir with mode 0o644 filtered by
// the umask (os.CreateTemp would force 0o600, which a Chmod could only widen
// past the umask). The name is prefix plus a decimal number.
func createAtomicTemp(dir, prefix string) (*os.File, error) {
	for range 1000 {
		name := filepath.Join(dir, prefix+strconv.FormatUint(uint64(rand.Uint32()), 10))
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, fmt.Errorf("create temp file in %s: too many name collisions", dir)
}

// atomicTarget resolves absPath to the real file the write must replace and
// re-confirms it is inside the vault and not restricted.
func (s *Service) atomicTarget(absPath string) (string, error) {
	var resolved string
	info, err := os.Lstat(absPath)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		// A dangling link fails here rather than being created through.
		resolved, err = filepath.EvalSymlinks(absPath)
		if err != nil {
			return "", err
		}
	case err == nil || errors.Is(err, os.ErrNotExist):
		dir, dirErr := filepath.EvalSymlinks(filepath.Dir(absPath))
		if dirErr != nil {
			return "", dirErr
		}
		resolved = filepath.Join(dir, filepath.Base(absPath))
		if err == nil {
			// A symlinked parent can still leave a symlink leaf unseen above only
			// if the leaf changed after the Lstat; resolve it again to be safe.
			if full, evalErr := filepath.EvalSymlinks(resolved); evalErr == nil {
				resolved = full
			}
		}
	default:
		return "", err
	}

	if !s.isUnderRoot(resolved) {
		return "", ErrSymlinkEscape
	}
	if rel, relErr := filepath.Rel(s.root, resolved); relErr == nil && s.restrictedRel(rel) {
		return "", ErrPathRestricted
	}
	return resolved, nil
}
