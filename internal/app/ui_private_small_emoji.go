package app

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

const (
	privateSmallEmojiTitle   = "多多西西"
	privateClassicEmojiTitle = "经典表情"
)

// Both sets share one URL table; the classic markers are also kept apart,
// as their pictures are smaller and square.
var privateSmallEmojiMarkers, privateClassicEmojiMarkers, privateSmallEmojiURLs = func() ([]string, []string, map[string]string) {
	urls := make(map[string]string, len(netease.DuoduoXixi)+len(netease.Classic))
	var small, classic []string
	for _, emoji := range netease.DuoduoXixi {
		small = append(small, emoji.Marker)
		urls[emoji.Marker] = emoji.URL
	}
	for _, emoji := range netease.Classic {
		classic = append(classic, emoji.Marker)
		urls[emoji.Marker] = emoji.URL
	}
	return small, classic, urls
}()

var privateClassicEmoji = func() map[string]bool {
	set := make(map[string]bool, len(privateClassicEmojiMarkers))
	for _, marker := range privateClassicEmojiMarkers {
		set[marker] = true
	}
	return set
}()

// The pattern finds both sets' markers; a bracketed word that is neither
// stays text.
var privateSmallEmojiPattern = func() *regexp.Regexp {
	names := make([]string, 0, len(privateClassicEmojiMarkers))
	for _, marker := range privateClassicEmojiMarkers {
		names = append(names, regexp.QuoteMeta(strings.Trim(marker, "[]")))
	}
	return regexp.MustCompile(`\[(?:(?:多多|西西)\p{Han}{2}|` + strings.Join(names, "|") + `)\]`)
}()

func (a *app) privateSmallEmojiBitmap(marker string) *ui.Bitmap {
	return bitmap(a.art.get(privateSmallEmojiURLs[marker], false))
}

func (a *app) privateMessageText(c *ui.Context, text string) {
	var spans []ui.Span
	var images map[int]*ui.Bitmap
	end, offset := 0, 0
	for _, at := range privateSmallEmojiPattern.FindAllStringIndex(text, -1) {
		marker := text[at[0]:at[1]]
		if privateSmallEmojiURLs[marker] == "" {
			continue // [多多…] names a picture only when it is one of the set
		}
		bmp := a.privateSmallEmojiBitmap(marker)
		if bmp == nil {
			continue // Keep the marker readable while its image loads.
		}
		if images == nil {
			images = make(map[int]*ui.Bitmap)
		}
		if plain := text[end:at[0]]; plain != "" {
			spans = append(spans, ui.Span{Text: plain})
			offset += utf8.RuneCountInString(plain)
		}
		images[offset] = bmp
		object := ui.Span{Text: "￼", ObjectWidth: 20, ObjectHeight: 16, ObjectDescent: 4}
		if privateClassicEmoji[marker] {
			// Square, and only 21 pixels as NetEase serves them.
			object.ObjectWidth, object.ObjectHeight = 18, 18
		}
		spans = append(spans, object)
		end, offset = at[1], offset+1
	}
	if len(spans) == 0 {
		ui.Text(c, text).FontSize(13).LineHeight(1.5)
		return
	}
	if end < len(text) {
		spans = append(spans, ui.Span{Text: text[end:]})
	}
	ui.RichText(c, spans...).FontSize(13).LineHeight(1.5).Label(text).
		DrawObjects(func(p *ui.Painter, at int, bounds ui.Rect) {
			p.Image(images[at], bounds, ui.Contain)
		})
}
