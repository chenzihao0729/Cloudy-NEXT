package app

import (
	"strings"
	"testing"
)

// Classic markers such as [大笑] are pictures beside the 多多西西 set, and a
// bracketed word that names neither stays text.
func TestPrivateClassicEmojiMarkers(t *testing.T) {
	if len(privateClassicEmojiMarkers) != 60 {
		t.Fatalf("%d classic markers, want 60", len(privateClassicEmojiMarkers))
	}
	for _, marker := range privateClassicEmojiMarkers {
		url := privateSmallEmojiURLs[marker]
		if !strings.HasPrefix(url, "https://s1.music.126.net/style/web2/emt/emoji_") || !strings.HasSuffix(url, ".png") {
			t.Errorf("%s: %q", marker, url)
		}
		if !privateClassicEmoji[marker] {
			t.Errorf("%s is not marked classic", marker)
		}
	}
	text := "[大笑][大笑]看[多多大笑]和[18][圈]，不是[大笑吧]也不是[歌名]"
	var found []string
	for _, at := range privateSmallEmojiPattern.FindAllStringIndex(text, -1) {
		found = append(found, text[at[0]:at[1]])
	}
	want := []string{"[大笑]", "[大笑]", "[多多大笑]", "[18]", "[圈]"}
	if strings.Join(found, ",") != strings.Join(want, ",") {
		t.Fatalf("found %v, want %v", found, want)
	}
	if privateClassicEmoji["[多多大笑]"] {
		t.Fatal("a 多多西西 marker was taken for a classic one")
	}
}
