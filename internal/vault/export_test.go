package vault

// SetAfterUnlockedReadHook installs a hook that RewriteLinksOnMove and
// RenameTag call after their unlocked scan read of a note and before the
// locked read-modify-write, letting a test interleave a concurrent write.
// The returned func restores the previous hook.
func SetAfterUnlockedReadHook(fn func(rel string)) (restore func()) {
	prev := afterUnlockedReadHook
	afterUnlockedReadHook = fn
	return func() { afterUnlockedReadHook = prev }
}

// SetBeforeRenameHook installs a hook that atomic writes call after the temp
// file is synced and before the rename; a non-nil return aborts the write.
// The returned func restores the previous hook.
func SetBeforeRenameHook(fn func() error) (restore func()) {
	prev := beforeRenameHook
	beforeRenameHook = fn
	return func() { beforeRenameHook = prev }
}
