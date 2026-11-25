package app

import (
	"crypto/sha1"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// The covers and avatars fetched from NetEase are kept on disk as well, in
// a pool of their own, apart from the songs' (「其他缓存」): kept only in
// memory, a cover let go of, or the app started again, was fetched again,
// and showed nothing until it came, over the page and as the app started
// (a viewer, 1.3.9). Read back, it shows at once.

// otherCacheFolderName is the folder they are kept in, inside one the
// listener chose for the cache, beside the songs'.
const otherCacheFolderName = "Cloudy-NEXT 其他缓存"

// otherCacheLimits are the sizes the pool may take, in megabytes.
var otherCacheLimits = []struct {
	mb    int
	label string
}{
	{0, "关闭"},
	{100, "100 MB"},
	{200, "200 MB"},
	{500, "500 MB"},
	{1024, "1 GB"},
}

// otherCacheDir is where they are kept: beside the songs, in the folder
// the listener chose, or else in the app's cache directory; "" without one.
func otherCacheDir(folder string) string {
	if folder != "" {
		return filepath.Join(folder, otherCacheFolderName)
	}
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "covers")
}

// artDisk keeps the bytes of pictures fetched, by URL, in files of a
// folder, up to limit bytes: those read or written least recently go first.
// Its methods may run on any goroutine.
type artDisk struct {
	mu    sync.Mutex
	dir   string
	limit int64
	size  int64 // what the files take, -1 until measured
	// pruning says a prune is on its way.
	pruning bool
}

func newArtDisk(dir string, limit int64) *artDisk {
	d := &artDisk{dir: dir, limit: limit, size: -1}
	go d.prune() // measures, and trims what a smaller limit left over
	return d
}

// path is where the bytes of url are kept: in one of 256 folders, by its
// hash, for no folder to grow too long.
func (d *artDisk) path(dir, url string) string {
	h := sha1.Sum([]byte(url))
	name := hex.EncodeToString(h[:])
	return filepath.Join(dir, name[:2], name)
}

// cacheable reports whether url is a picture from the web, which the pool
// keeps: a local file's is at hand already.
func cacheable(url string) bool {
	return strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")
}

// read returns the bytes kept of url, marking them used now.
func (d *artDisk) read(url string) ([]byte, bool) {
	if d == nil || !cacheable(url) {
		return nil, false
	}
	d.mu.Lock()
	dir, on := d.dir, d.limit > 0
	d.mu.Unlock()
	if dir == "" || !on {
		return nil, false
	}
	p := d.path(dir, url)
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	now := time.Now()
	os.Chtimes(p, now, now)
	return b, true
}

// write keeps b, the bytes of url, and trims the pool once it is over its
// limit.
func (d *artDisk) write(url string, b []byte) {
	if d == nil || !cacheable(url) || len(b) == 0 {
		return
	}
	d.mu.Lock()
	dir, limit := d.dir, d.limit
	d.mu.Unlock()
	if dir == "" || limit <= 0 || int64(len(b)) > limit/8 {
		return
	}
	p := d.path(dir, url)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	if writeFileAtomic(p, b, 0o600) != nil {
		return
	}
	d.mu.Lock()
	over := false
	if d.size >= 0 {
		d.size += int64(len(b))
		over = d.size > d.limit && !d.pruning
	}
	if over {
		d.pruning = true
	}
	d.mu.Unlock()
	if over {
		go d.prune()
	}
}

// prune measures the pool, and drops the files used least recently until
// it is within nine tenths of its limit.
func (d *artDisk) prune() {
	d.mu.Lock()
	dir, limit := d.dir, d.limit
	d.pruning = true
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.pruning = false
		d.mu.Unlock()
	}()
	type file struct {
		path string
		size int64
		used time.Time
	}
	var files []file
	var total int64
	if dir != "" {
		filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			info, err := e.Info()
			if err != nil {
				return nil
			}
			if strings.HasSuffix(e.Name(), ".part") {
				if time.Since(info.ModTime()) > time.Minute {
					os.Remove(path) // left by a write cut short
				}
				return nil
			}
			files = append(files, file{path, info.Size(), info.ModTime()})
			total += info.Size()
			return nil
		})
	}
	if total > limit {
		slices.SortFunc(files, func(x, y file) int { return x.used.Compare(y.used) })
		keep := limit * 9 / 10
		for _, f := range files {
			if total <= keep {
				break
			}
			if os.Remove(f.path) == nil {
				total -= f.size
			}
		}
	}
	d.mu.Lock()
	if d.dir == dir {
		d.size = total
	}
	d.mu.Unlock()
}

// Size returns what the pool takes, measuring it if it has not yet.
func (d *artDisk) Size() int64 {
	d.mu.Lock()
	n := d.size
	d.mu.Unlock()
	if n < 0 {
		d.prune()
		d.mu.Lock()
		n = d.size
		d.mu.Unlock()
	}
	return n
}

// SetLimit keeps the pool within limit bytes from now; 0 keeps nothing
// more, and drops what is kept.
func (d *artDisk) SetLimit(limit int64) {
	d.mu.Lock()
	d.limit = limit
	d.mu.Unlock()
	d.prune()
}

// SetDir keeps the pool in dir from now, dropping what was kept where it
// was, as the songs' cache does as it moves.
func (d *artDisk) SetDir(dir string) {
	d.mu.Lock()
	old := d.dir
	d.dir, d.size = dir, -1
	d.mu.Unlock()
	if old != "" && old != dir {
		removeOtherCache(old)
	}
	d.prune()
}

// Clear drops all that the pool keeps.
func (d *artDisk) Clear() {
	d.mu.Lock()
	dir := d.dir
	d.mu.Unlock()
	if dir != "" {
		removeOtherCache(dir)
	}
	d.mu.Lock()
	if d.dir == dir {
		d.size = 0
	}
	d.mu.Unlock()
}

// removeOtherCache deletes the pool's files in dir: its 256 folders of
// hashes and what they hold, nothing else, should the folder be one the
// listener put other things in.
func removeOtherCache(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if n := e.Name(); e.IsDir() && len(n) == 2 && isHex(n) {
			os.RemoveAll(filepath.Join(dir, n))
		}
	}
	os.Remove(dir) // if it is empty now
}

func isHex(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
