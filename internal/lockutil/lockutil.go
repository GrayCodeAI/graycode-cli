// Package lockutil provides the platform-specific file primitives behind the
// O_EXCL lock files used by hawk (daemon, hooks, cron, install): a no-overwrite
// restore for locks that were sidelined during a stale-reclaim attempt, and a
// lock file remover with one cross-platform contract (missing files are a no-op;
// Windows retries transient sharing violations).
//
// Adopted from Zero (internal/lockutil) — atomic stale-lock reclaim primitives.
package lockutil

import (
	"io"
	"os"
)

// restoreByCopy restores reclaimed to path without overwriting an existing
// path, as a fallback for when the platform's primary no-replace primitive
// (hard link on POSIX, MoveFileEx on Windows) fails for a reason other than
// the destination existing. It stages a full copy under a private name next
// to reclaimed and publishes it to path with publish (the same no-replace
// primitive the caller's platform uses for the primary restore), so path
// never appears in a partially-copied state. publish keeps the no-overwrite
// guarantee: a new holder that appeared in the meantime wins and this returns
// os.ErrExist. The copy resets the lock's mtime to now, which only makes the
// restored lock look fresher; that is safe, since it is being handed back to
// a live holder.
func restoreByCopy(reclaimed, path string, publish func(from, to string) error) error {
	staged := reclaimed + ".copy"
	src, err := os.Open(reclaimed)
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(staged, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = src.Close()
		return err
	}
	_, err = io.Copy(dst, src)
	// Close the source before removing anything below: Go opens files without
	// FILE_SHARE_DELETE on Windows, so deleting reclaimed or staged while src
	// is open would fail with a sharing violation.
	_ = src.Close()
	if err != nil {
		_ = dst.Close()
		_ = os.Remove(staged)
		return err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(staged)
		return err
	}
	if err := publish(staged, path); err != nil {
		_ = os.Remove(staged)
		return err
	}
	_ = os.Remove(staged)
	_ = RemoveLockFile(reclaimed)
	return nil
}
