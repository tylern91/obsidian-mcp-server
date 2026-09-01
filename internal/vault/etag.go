package vault

import (
	"crypto/sha256"
	"encoding/hex"
)

// Etag returns the hex-encoded SHA-256 digest of data. Every etag emitted by
// read_note/get_notes_info/read_multiple_notes and every if_match comparison
// on a mutating call must go through this single function.
func Etag(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// WriteOpt configures optional optimistic-concurrency behavior on a mutating
// vault call.
type WriteOpt func(*writeOpts)

type writeOpts struct {
	ifMatch string
}

// WithIfMatch makes a mutating call conditional on the note's current
// content hashing to etag (per Etag). An empty etag means "no condition".
func WithIfMatch(etag string) WriteOpt {
	return func(o *writeOpts) { o.ifMatch = etag }
}

func applyWriteOpts(opts []WriteOpt) writeOpts {
	var o writeOpts
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// checkIfMatch compares data's etag against opts.ifMatch when a condition
// was supplied, returning ErrRevisionConflict on mismatch. Must be called
// while holding s.mu, immediately after the read whose bytes are being
// compared, so the check and the write it guards can't race.
func checkIfMatch(op, path string, data []byte, opts writeOpts) error {
	if opts.ifMatch == "" {
		return nil
	}
	if Etag(data) != opts.ifMatch {
		return &PathError{Op: op, Path: path, Err: ErrRevisionConflict}
	}
	return nil
}
