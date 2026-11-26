package netease

import (
	"fmt"
	"net/url"
)

// Ranked is a song of the listener's ranking: how many times they played
// it, and NetEase's score of it, 100 for the most played.
type Ranked struct {
	Track Track
	Plays int
	Score int
}

// WeekRanking returns the songs the listener uid played most in the last
// seven days, the most played first: NetEase's 听歌排行 of the week. Its
// days are Beijing's, from midnight seven days ago until now.
func WeekRanking(uid int64) ([]Ranked, error) {
	return ranking(uid, "1")
}

// AllRanking returns the songs the listener uid played most of all time,
// a hundred at most, the most played first: NetEase's 听歌排行 of 所有时间.
func AllRanking(uid int64) ([]Ranked, error) {
	return ranking(uid, "0")
}

func ranking(uid int64, kind string) ([]Ranked, error) {
	type row struct {
		PlayCount int   `json:"playCount"`
		Score     int   `json:"score"`
		Song      Track `json:"song"`
	}
	var res struct {
		WeekData []row `json:"weekData"`
		AllData  []row `json:"allData"`
	}
	form := url.Values{"uid": {fmt.Sprint(uid)}, "type": {kind}}
	if err := post("/api/v1/play/record", form, &res); err != nil {
		return nil, err
	}
	rows := res.WeekData
	if kind == "0" {
		rows = res.AllData
	}
	out := make([]Ranked, len(rows))
	songs := make([]Track, len(out))
	for i, w := range rows {
		songs[i] = w.Song
		out[i] = Ranked{Plays: w.PlayCount, Score: w.Score}
	}
	// They come without privileges, as the daily songs do.
	detailPrivileges(songs)
	for i := range out {
		out[i].Track = songs[i]
	}
	return out, nil
}

// ListenTotal is what NetEase says of the listener's listening on their
// page: the songs they listened to, all told, and how many days ago they
// joined.
type ListenTotal struct {
	Songs int
	Days  int
}

// Listened returns the listener uid's ListenTotal.
func Listened(uid int64) (ListenTotal, error) {
	var res struct {
		ListenSongs int `json:"listenSongs"`
		CreateDays  int `json:"createDays"`
	}
	if err := get("/api/v1/user/detail/"+fmt.Sprint(uid), nil, &res); err != nil {
		return ListenTotal{}, err
	}
	return ListenTotal{Songs: res.ListenSongs, Days: res.CreateDays}, nil
}
