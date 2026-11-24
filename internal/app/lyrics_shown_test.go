package app

import (
	"testing"

	"netease-music-mygo/internal/lyrics"
)

// A piece NetEase gives only 纯音乐，请欣赏 for, with who made it or not, has
// no lyrics to show; one that says it in a line among others has them.
func TestPureMusicHasNoLines(t *testing.T) {
	for _, c := range []struct {
		lines []lyrics.Line
		pure  bool
	}{
		{[]lyrics.Line{{Text: "纯音乐，请欣赏"}}, true},
		{[]lyrics.Line{{Text: "作曲 : 所长sama"}, {Text: "纯音乐，请欣赏"}}, true},
		{[]lyrics.Line{{Text: "纯音乐，请欣赏"}, {Text: "风从海面吹过来"}}, false},
		{[]lyrics.Line{{Text: "作曲 : 所长sama"}}, false},
		{nil, false},
	} {
		if got := pureMusic(c.lines); got != c.pure {
			t.Errorf("%v: pure %v, want %v", c.lines, got, c.pure)
		}
	}
}
