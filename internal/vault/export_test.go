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
