// Package fastcopy copies directory trees using copy-on-write where the
// filesystem supports it (APFS clonefile on macOS, FICLONE ioctl on Linux),
// falling back to plain byte copies elsewhere. Parallelism is sharded by
// parent directory so files in the same directory always land on the same
// worker, avoiding create-dir lock contention — the technique grok-build's
// xai-fast-worktree uses to make standalone worktree creation O(file_count)
// instead of a serial walk.
package fastcopy

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// maxWorkers bounds the copy worker pool. macOS defaults to a low soft
// file-descriptor limit, so it gets fewer workers than Linux.
func maxWorkers() int {
	if runtime.GOOS == "darwin" {
		return 8
	}
	return 32
}

// shardCount is the number of work shards. Prime counts distribute better.
const shardCount = 16

func shardFor(dir string) int {
	sum := sha256.Sum256([]byte(dir))
	return int(binary.BigEndian.Uint32(sum[:4]) % shardCount)
}

// Tree copies the directory tree at src to dst. Existing dst content is
// overwritten file-by-file. Returns the number of files copied and bytes
// written. ctx cancellation stops the walk at the next file.
func Tree(ctx context.Context, src, dst string) (files int64, bytes int64, err error) {
	info, err := os.Stat(src)
	if err != nil {
		return 0, 0, fmt.Errorf("fastcopy: stat src: %w", err)
	}
	if !info.IsDir() {
		return 0, 0, fmt.Errorf("fastcopy: src is not a directory: %s", src)
	}

	type job struct{ rel string }
	jobs := make([][]job, shardCount)

	walkErr := filepath.WalkDir(src, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o750)
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks/devices/fifos in workspace copies
		}
		s := shardFor(filepath.Dir(rel))
		jobs[s] = append(jobs[s], job{rel: rel})
		return nil
	})
	if walkErr != nil {
		return atomic.LoadInt64(&files), atomic.LoadInt64(&bytes), walkErr
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		fileCnt  atomic.Int64
		byteCnt  atomic.Int64
	)
	for s := 0; s < shardCount; s++ {
		shardJobs := jobs[s]
		if len(shardJobs) == 0 {
			continue
		}
		wg.Add(1)
		go func(jobs []job) {
			defer wg.Done()
			for _, j := range jobs {
				if ctx.Err() != nil {
					return
				}
				n, err := copyFileCoW(
					filepath.Join(src, j.rel),
					filepath.Join(dst, j.rel),
				)
				fileCnt.Add(1)
				byteCnt.Add(n)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
			}
		}(shardJobs)
		if s+1 >= maxWorkers() {
			// Reuse finished workers before spawning more shards.
			wg.Wait()
		}
	}
	wg.Wait()
	if firstErr != nil {
		return fileCnt.Load(), byteCnt.Load(), firstErr
	}
	return fileCnt.Load(), byteCnt.Load(), ctx.Err()
}

// copyFileCoW clones src into dst via CoW when possible, else a byte copy.
func copyFileCoW(src, dst string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return 0, err
	}
	if n, cloned := tryCloneFile(src, dst); cloned {
		return n, nil
	}
	in, err := os.Open(src) // #nosec G304 -- caller-supplied tree paths
	if err != nil {
		return 0, err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) // #nosec G304 -- mirrored tree path
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, in)
	cerr := out.Close()
	if err == nil {
		err = cerr
	}
	return n, err
}

// tryCloneFile attempts a filesystem-level clone; returns cloned=false when
// unsupported (caller falls back to a byte copy, which stays correct
// everywhere). The per-platform implementations live in clone_darwin.go and
// clone_other.go.
func tryCloneFile(src, dst string) (int64, bool) {
	return tryCloneFilePlatform(src, dst)
}

// SupportsCloneFile reports whether a quick probe clone succeeds on the
// filesystem containing dir (used by callers to log/choose strategies).
func SupportsCloneFile(dir string) bool {
	probeSrc := filepath.Join(dir, ".hawk-fastcopy-probe")
	if err := os.WriteFile(probeSrc, []byte("probe"), 0o600); err != nil {
		return false
	}
	defer func() { _ = os.Remove(probeSrc) }()
	probeDst := probeSrc + ".clone"
	defer func() { _ = os.Remove(probeDst) }()
	_, ok := tryCloneFile(probeSrc, probeDst)
	return ok
}

// TrimPrefixPath is a tiny helper for logging relative paths without leaking
// absolute prefixes.
func TrimPrefixPath(root, p string) string {
	return strings.TrimPrefix(strings.TrimPrefix(p, root), string(filepath.Separator))
}
