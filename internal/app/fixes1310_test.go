package app

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Deleting the listener's own comment, first of the latest as new ones are, takes it out
// of the list as the rows are built: the rows go on over the list as it was,
// where they read past its new end and crashed the app (1.3.9, every time).
func TestDeleteOwnCommentKeepsTheApp(t *testing.T) {
	asked := make(chan int64, 2)
	deleteComment = func(thread string, id int64) error { asked <- id; return nil }
	defer func() { deleteComment = netease.DeleteComment }()
	a, tt, _ := coverApp(t)
	a.profile = &netease.Profile{UserID: 1}
	other := netease.Comment{ID: 8, Content: "好听"}
	other.User.ID, other.User.Nickname = 2, "别人"
	own := netease.Comment{ID: 9, Content: "我的"}
	own.User.ID, own.User.Nickname = 1, "我"
	a.comments = commentState{track: 2, thread: "R_SO_4_2", loaded: true, total: 2,
		hot: []netease.Comment{other}, latest: []netease.Comment{own, other}}
	a.npComments = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("删除评论"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("确认删除评论"); err != nil {
		t.Fatal(err)
	}
	if id := wait(t, asked, "the delete"); id != 9 {
		t.Errorf("asked NetEase to delete %d", id)
	}
	tt.Frame()
	if len(a.comments.latest) != 1 || a.comments.latest[0].ID != 8 || len(a.comments.hot) != 1 || a.comments.total != 1 {
		t.Errorf("left latest %v, hot %v, total %d", a.comments.latest, a.comments.hot, a.comments.total)
	}
}

// A queue kept from a run while the membership had lapsed says its VIP
// songs may not be played: restored, NetEase is asked again about those,
// and what it answers now lets them play (1.3.9: renewed, they stayed grey).
func TestRestoredQueueAsksAgainWhatPlays(t *testing.T) {
	isolateSessionPath(t)
	vip := netease.Track{ID: 7, Name: "会员歌", Duration: 200000, Privilege: &netease.Privilege{ID: 7, Pl: 0}}
	free := netease.Track{ID: 8, Name: "免费歌", Duration: 200000, Privilege: &netease.Privilege{ID: 8, Pl: 320000}}
	a := newTestApp()
	a.pl.Cue([]netease.Track{free, vip})
	a.saveSession()

	asked := make(chan []int64, 1)
	privilegeDetails = func(ids []int64) ([]netease.Track, error) {
		asked <- ids
		return []netease.Track{{ID: 7, Privilege: &netease.Privilege{ID: 7, Pl: 320000}}}, nil
	}
	defer func() { privilegeDetails = netease.SongDetails }()
	b := newTestApp()
	b.profile = &netease.Profile{UserID: 1}
	b.restoreSession()
	ids := wait(t, asked, "the privileges")
	if len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("asked about %v, not just the grey song", ids)
	}
	// What the answer does on the main thread, which tests have none of.
	b.privilegesCame(b.accountGen, nil, map[int64]netease.Privilege{7: {ID: 7, Pl: 320000}}, nil)
	for _, tr := range b.pl.Queue() {
		if !tr.Playable() {
			t.Errorf("%s still grey", tr.Name)
		}
	}
	// Signed in as someone else since, an answer is not theirs.
	c := newTestApp()
	c.pl.Cue([]netease.Track{vip})
	c.privilegesCame(c.accountGen+1, nil, map[int64]netease.Privilege{7: {ID: 7, Pl: 320000}}, nil)
	if c.pl.Queue()[0].Playable() {
		t.Error("took another login's answer")
	}
}

// A cover kept on disk comes back from there, without the network; the
// pool keeps within its limit, those used least recently going first, and
// moving or clearing it leaves nothing else of the folder.
func TestCoversKeptOnDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "covers")
	// Keep initial measurement synchronous: background pruning during
	// setup could leave the pool below its limit before the explicit
	// prune, so it correctly skipped the test's expected 90% trim.
	d := &artDisk{dir: dir, limit: 100 << 10, size: -1}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	png.Encode(&buf, img)
	url := "https://p1.music.126.net/never-fetched.jpg?param=96y96"
	d.write(url, buf.Bytes())
	a, err := loadArtVia(d, url, false, false, false)
	if err != nil || a == nil || a.bmp == nil {
		t.Fatalf("not read back from disk: %v", err)
	}
	if _, ok := d.read("C:/music/a.jpg"); ok {
		t.Error("kept a local file's picture")
	}

	// Over the limit, the oldest go, down to nine tenths of it.
	chunk := bytes.Repeat([]byte{1}, 10<<10)
	for i := range 12 {
		d.write(fmt.Sprintf("https://x/%d", i), chunk)
		old := time.Now().Add(time.Duration(i-20) * time.Minute)
		os.Chtimes(d.path(dir, fmt.Sprintf("https://x/%d", i)), old, old)
	}
	d.prune()
	if n := d.Size(); n > 90<<10 {
		t.Errorf("pool takes %d bytes", n)
	}
	if _, ok := d.read("https://x/0"); ok {
		t.Error("the oldest kept")
	}
	if _, ok := d.read("https://x/11"); !ok {
		t.Error("the newest dropped")
	}

	// Its folder may hold the listener's own files: they stay.
	own := filepath.Join(dir, "notes.txt")
	os.WriteFile(own, []byte("mine"), 0o644)
	d.Clear()
	if _, ok := d.read("https://x/11"); ok {
		t.Error("kept after clearing")
	}
	if _, err := os.Stat(own); err != nil {
		t.Error("cleared the listener's file")
	}
	to := filepath.Join(t.TempDir(), "elsewhere")
	d.write("https://x/a", chunk)
	d.SetDir(to)
	if _, ok := d.read("https://x/a"); ok {
		t.Error("moved, still found in the old folder")
	}
	d.write("https://x/b", chunk)
	if _, ok := d.read("https://x/b"); !ok {
		t.Error("not kept in the new folder")
	}
	d.SetLimit(0)
	if _, ok := d.read("https://x/b"); ok || d.Size() != 0 {
		t.Error("turned off, still kept")
	}
}

// The sidebar lists what the listener chose, in their order: hidden from
// its own menu, the entry goes; 为你推荐 cannot be; an order saved before
// an entry was added keeps it, where it comes by default.
func TestSidebarAsChosen(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("排行榜"); !ok {
		t.Fatal("no 排行榜 to start with")
	}
	if err := tt.RightClick("排行榜"); err != nil {
		t.Fatal(err)
	}
	if err := chooseItem(tt, "从侧栏隐藏「排行榜」"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if _, ok := tt.Find("排行榜"); ok || a.settings.sideOn("charts") {
		t.Error("排行榜 still shown")
	}
	a.setSideShown("home", false)
	if !a.settings.sideOn("home") {
		t.Error("为你推荐 hidden")
	}
	// Its menu offers no hiding, only editing, which opens the settings
	// with the list open.
	tt.Frame()
	if err := tt.RightClick("为你推荐"); err != nil {
		t.Fatal(err)
	}
	if err := chooseItem(tt, "编辑侧栏…"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsView.open || a.settingsView.tab != settingsHome || !a.sideDrawer {
		t.Errorf("settings open %v at %d, list open %v", a.settingsView.open, a.settingsView.tab, a.sideDrawer)
	}

	// An order kept from before 我的专辑 was known puts it after 收藏的歌单.
	s := settings{SideOrder: []string{"recent", "home", "mine", "saved", "fm"}}
	order := s.sideOrder()
	if len(order) != len(sideItems) || order[0] != "recent" || slices.Index(order, "albums") != slices.Index(order, "saved")+1 {
		t.Errorf("order %v", order)
	}

	// Reordered, the hidden stay where they were.
	b := newTestApp()
	b.settings = defaultSettings()
	b.settings.SideHidden = []string{"podcasts"}
	shown := []string{}
	for _, id := range b.settings.sideOrder() {
		if b.settings.sideOn(id) {
			shown = append(shown, id)
		}
	}
	shown[0], shown[1] = shown[1], shown[0] // 私人 FM over 为你推荐
	b.sideReorder(shown)
	got := b.settings.sideOrder()
	if got[0] != "fm" || got[1] != "home" || got[3] != "podcasts" {
		t.Errorf("reordered %v", got)
	}

	// Folded by default, a group starts folded.
	b.setSideFolded("mine", true)
	c := newTestApp()
	c.settings = b.settings
	c.sideFold()
	if !c.mineCollapsed || c.savedCollapsed {
		t.Errorf("folded mine %v, saved %v", c.mineCollapsed, c.savedCollapsed)
	}
}

// A song downloaded, or a file found on NetEase, has its heart; a file
// NetEase knows nothing of has none.
func TestDownloadedSongsHaveHearts(t *testing.T) {
	if heartless(&netease.Track{ID: 5, LocalPath: `D:\Music\a.mp3`}) {
		t.Error("a downloaded song has no heart")
	}
	if !heartless(&netease.Track{ID: -3, LocalPath: `D:\Music\b.mp3`}) || !heartless(&netease.Track{LocalPath: `D:\Music\c.mp3`}) {
		t.Error("a file of the listener's own has a heart")
	}
}

// Standing, an English line lies word by word, each word a piece, so that
// a long one goes on into the next column rather than being cut short with
// an ellipsis, as one piece longer than the column was (1.3.9).
func TestStandingEnglishWraps(t *testing.T) {
	var pieces []deskPiece
	tt := ui.NewTester(func(c *ui.Context) {
		pieces = deskPieces([]rune("I don't want to play around 我们"), 32, 800, "")
	}, 300, 300)
	tt.Frame()
	var words []string
	for _, p := range pieces {
		if p.sideways {
			words = append(words, p.text)
		}
	}
	if !slices.Equal(words, []string{"I", "don't", "want", "to", "play", "around"}) {
		t.Errorf("words %q", words)
	}
	room := float32(deskVH - deskToolsRoom - 8)
	for _, p := range pieces {
		if p.h > room {
			t.Errorf("%q longer than a column", p.text)
		}
	}
}

// A long line slides into its ends gently: no faster at an end than in
// between, and at rest there.
func TestDeskSlideEasesAtTheEnds(t *testing.T) {
	const run, d = 400, 60
	if softClamp(-200, run, d) != 0 || softClamp(900, run, d) != run || softClamp(200, run, d) != 200 {
		t.Fatal("not held within its ends")
	}
	prev := softClamp(-d, run, d)
	for x := float32(-d) + 1; x <= run+d; x++ {
		v := softClamp(x, run, d)
		if step := v - prev; step < -0.001 || step > 1.001 {
			t.Fatalf("at %v it moved %v", x, step)
		}
		prev = v
	}
	if near := softClamp(run+d-1, run, d); run-near > 0.01 {
		t.Errorf("still %v from the end just before it", run-near)
	}
}
