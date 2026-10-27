// Package blob stores immutable files addressed by their sha256 hex digest under
// <dir>/sha256/<first two hex chars>/<digest>.
package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Store struct {
	root string
	// mu orders Put's existence check against GC's age check and removal, so a blob
	// re-used by Put is either refreshed before GC looks at it or rewritten after.
	mu sync.Mutex
}

func Open(dir string) (*Store, error) {
	root := filepath.Join(dir, "sha256")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ValidHash(hash string) bool { return hashPattern.MatchString(hash) }

func (s *Store) path(hash string) string {
	return filepath.Join(s.root, hash[:2], hash)
}

// Put writes data unless a blob with the same digest already exists, in which case it
// refreshes the file's mtime so GC treats it as new until the caller commits its reference.
func (s *Store) Put(data []byte) (string, error) {
	hash := Hash(data)
	p := s.path(hash)
	if s.touch(p) {
		return hash, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), hash+".*.tmp")
	if err != nil {
		return "", err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		s.mu.Lock()
		err = os.Rename(tmp.Name(), p)
		s.mu.Unlock()
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return hash, nil
}

func (s *Store) touch(p string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	return os.Chtimes(p, now, now) == nil
}

func (s *Store) Read(hash string) ([]byte, error) {
	if !ValidHash(hash) {
		return nil, fmt.Errorf("invalid blob hash")
	}
	return os.ReadFile(s.path(hash))
}

func (s *Store) Exists(hash string) bool {
	if !ValidHash(hash) {
		return false
	}
	_, err := os.Stat(s.path(hash))
	return err == nil
}

// GC removes blobs and temporary files older than minAge that keep(hash) rejects. The
// age guard keeps blobs written by requests that have not committed their rows yet.
func (s *Store) GC(keep func(string) bool, minAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-minAge)
	removed := 0
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".tmp") && (!ValidHash(name) || keep(name)) {
			return nil
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if info, err := os.Stat(p); err == nil && info.ModTime().Before(cutoff) && os.Remove(p) == nil {
			removed++
		}
		return nil
	})
	return removed, err
}
