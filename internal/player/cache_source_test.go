package player

import (
	"os"
	"path/filepath"
	"testing"

	"netease-music-mygo/internal/netease"
)

// A cached QQ stream stays namespaced from NetEase audio and remains discoverable after restart.
func TestCacheSeparatesAlternateSource(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 1<<20)
	primaryPart := filepath.Join(dir, "primary.part")
	if err := os.WriteFile(primaryPart, []byte("netease"), 0o600); err != nil {
		t.Fatal(err)
	}
	primary := &netease.Stream{Type: "mp3", Level: "exhigh", Size: int64(len("netease"))}
	if _, ok := c.keep(7, primary, primaryPart, primary.Size); !ok {
		t.Fatal("NetEase stream wasn't cached")
	}
	alternatePart := filepath.Join(dir, "alternate.part")
	if err := os.WriteFile(alternatePart, []byte("qqmusic"), 0o600); err != nil {
		t.Fatal(err)
	}
	alternate := &netease.Stream{Type: "mp3", Level: "exhigh", Size: int64(len("qqmusic")), Source: "qq", SourceID: "song-mid"}
	if _, ok := c.keep(7, alternate, alternatePart, alternate.Size); !ok {
		t.Fatal("QQ stream wasn't cached")
	}
	if filepath.Base(cacheKeyFor(7, alternate)) == cacheKey(7, "exhigh") {
		t.Fatal("alternate source shares the NetEase cache key")
	}
	dl, st, ok := c.lookup(7, "exhigh")
	if !ok || st.Source != "" {
		t.Fatalf("lookup with both entries = %#v, %v; want NetEase first", st, ok)
	}
	dl.Close()
	if err := os.Remove(filepath.Join(dir, cacheKey(7, "exhigh")+".mp3")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, cacheKey(7, "exhigh")+".json")); err != nil {
		t.Fatal(err)
	}
	dl, st, ok = c.lookup(7, "exhigh")
	if !ok || st.Source != "qq" || st.SourceID != "song-mid" {
		t.Fatalf("lookup after primary removal = %#v, %v; want cached QQ stream", st, ok)
	}
	dl.Close()
}
