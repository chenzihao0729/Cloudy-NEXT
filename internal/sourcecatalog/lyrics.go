package sourcecatalog

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"netease-music-mygo/internal/qqmusic"
	"strconv"
	"strings"
)

// Lyrics fetches public lyrics for the exact resolved provider recording.
func Lyrics(ctx context.Context, platform, id string) (string, error) {
	switch platform {
	case "qq", "tx":
		return qqmusic.Lyrics(ctx, id)
	case "kw":
		var data struct {
			Data struct {
				Lines []struct {
					Time string `json:"time"`
					Text string `json:"lineLyric"`
				} `json:"lrclist"`
			} `json:"data"`
		}
		err := getJSON(ctx, "https://m.kuwo.cn/newh5/singles/songinfoandlrc?musicId="+url.QueryEscape(id), "https://m.kuwo.cn/", &data)
		if err != nil {
			return "", err
		}
		var out strings.Builder
		for _, line := range data.Data.Lines {
			seconds, err := strconv.ParseFloat(line.Time, 64)
			if err != nil || seconds < 0 {
				continue
			}
			fmt.Fprintf(&out, "[%02d:%05.2f]%s\n", int(seconds)/60, seconds-float64(int(seconds)/60*60), line.Text)
		}
		return out.String(), nil
	case "kg":
		var found struct {
			Candidates []struct {
				ID  string `json:"id"`
				Key string `json:"accesskey"`
			} `json:"candidates"`
		}
		if err := getJSON(ctx, "https://lyrics.kugou.com/search?ver=1&man=yes&client=pc&hash="+url.QueryEscape(id), "https://www.kugou.com/", &found); err != nil {
			return "", err
		}
		if len(found.Candidates) == 0 {
			return "", nil
		}
		candidate := found.Candidates[0]
		var result struct {
			Content string `json:"content"`
		}
		if err := getJSON(ctx, "https://lyrics.kugou.com/download?ver=1&client=pc&fmt=lrc&charset=utf8&id="+url.QueryEscape(candidate.ID)+"&accesskey="+url.QueryEscape(candidate.Key), "https://www.kugou.com/", &result); err != nil {
			return "", err
		}
		b, err := base64.StdEncoding.DecodeString(result.Content)
		return string(b), err
	case "mg":
		var result struct {
			Lyric string `json:"lyric"`
		}
		err := getJSON(ctx, "https://music.migu.cn/v3/api/music/audioPlayer/getLyric?copyrightId="+url.QueryEscape(id), "https://music.migu.cn/", &result)
		return result.Lyric, err
	}
	return "", nil
}
