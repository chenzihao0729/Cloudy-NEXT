package app

import (
	"runtime/debug"
	"runtime/metrics"
	"time"

	"github.com/egoist/mygo"
)

// Left to itself, Go collects an idle heap only every two minutes, and
// gives the pages it frees back to Windows slowly, keeping a heap's worth
// more than it uses; the renderer frees a canvas's texture only once the
// canvas is collected. So what a page or a view took stays counted
// against the app long after it goes. debug.FreeOSMemory collects at once
// and hands the free pages back: the app calls it when it is quiet.
const (
	// releaseDelay is how long after a heavy view goes (Now Playing
	// closed, the desk lyrics or the mini player shut) that the memory
	// goes back: past the frames that let go of it, and late enough that
	// turning it on and off again in quick turns pays once.
	releaseDelay = 1500 * time.Millisecond
	// hiddenDelay is how long after the window hides or minimizes: past
	// the two seconds after which its renderer frees its frame.
	hiddenDelay = 2500 * time.Millisecond
	// blurDelay is how long after the window loses the focus to another.
	blurDelay = 3 * time.Second
	// pageDelay is how long after the last change of page, browsing having
	// stopped there.
	pageDelay = 5 * time.Second
	// quietEvery is how often the app looks whether memory it no longer
	// uses has piled up without any of the above, as when it is left open
	// and playing for hours.
	quietEvery = 30 * time.Second
	// quietGap and eventGap are how long after memory last went back that
	// such a look, or a blur or page change, may send it back again.
	quietGap = time.Minute
	eventGap = 15 * time.Second
	// quietWorth and eventWorth are the least memory that sending back
	// takes, for a look or for a blur or page change to do it.
	quietWorth = 32 << 20
	eventWorth = 16 << 20
)

// memRelease is when memory last went back, and the release on its way.
type memRelease struct {
	t       *time.Timer
	pending bool
	worth   uint64 // the least memory the release on its way needs to get back; 0, any
	last    time.Time
	count   int // how many times memory went back, for the mem scene
}

// releaseSoon gives back the memory of a view just gone, once
// releaseDelay passes with no other release asked for. Call it on the
// main thread, as all of these.
func (a *app) releaseSoon() { a.releaseAfter(releaseDelay, 0) }

// releaseAfter gives back the memory the app no longer uses, once d
// passes with no other release asked for, if at least worth bytes of it
// would go (any for 0) and, unless worth is 0, eventGap passed since it
// last went back.
func (a *app) releaseAfter(d time.Duration, worth uint64) {
	m := &a.mem
	if m.pending {
		m.worth = min(m.worth, worth)
	} else {
		m.pending, m.worth = true, worth
	}
	if a.win == nil {
		// No window, no main thread to run it on: as in tests.
		return
	}
	if m.t == nil {
		m.t = time.AfterFunc(d, func() { mygo.RunOnMain(a.releaseDue) })
	} else {
		m.t.Reset(d)
	}
}

// releaseDue runs the release on its way, when its time comes.
func (a *app) releaseDue() {
	m := &a.mem
	if !m.pending {
		return
	}
	if a.npMoving() {
		// Opening or closing Now Playing allocates as it goes: once it
		// stops.
		if m.t != nil {
			m.t.Reset(time.Second)
		}
		return
	}
	m.pending = false
	if m.worth > 0 && (time.Since(m.last) < eventGap || reclaimable() < m.worth) {
		return
	}
	a.freeOSMemory()
}

// quietCheck gives back the memory piled up, unused, when quietEvery
// finds quietWorth of it quietGap after it last went back.
func (a *app) quietCheck() {
	m := &a.mem
	if m.pending || a.npMoving() || time.Since(m.last) < quietGap || reclaimable() < quietWorth {
		return
	}
	a.freeOSMemory()
}

// watchQuiet has quietCheck look every quietEvery for as long as the app
// runs.
func (a *app) watchQuiet() {
	go func() {
		for range time.Tick(quietEvery) {
			mygo.RunOnMain(a.quietCheck)
		}
	}()
}

// releaseHidden lets go of what a hidden window has no use for, then gives
// the memory back: hidden to the tray, the covers asked for least recently
// beyond hiddenArtLimit too, as it may stay hidden for hours.
func (a *app) releaseHidden(tray bool) {
	if tray {
		a.art.trim(hiddenArtLimit)
	}
	a.trimPageCaches()
	a.trimAutoCache()
	a.releaseAfter(hiddenDelay, 0)
}

// hiddenArtLimit is how many bytes of covers the cache keeps while the
// window is hidden to the tray.
const hiddenArtLimit = 8 << 20

// npMoving reports whether Now Playing is opening or closing.
func (a *app) npMoving() bool { return a.npK > 0.001 && a.npK < 0.999 }

func (a *app) freeOSMemory() {
	a.mem.last = time.Now()
	a.mem.count++
	// Off the main thread: collecting takes a few milliseconds that the
	// window's frames need not wait for.
	go debug.FreeOSMemory()
}

// reclaimable is how many bytes of the heap a collection would free and
// give back: the garbage made since the last one, and the pages freed
// that Go still holds.
func reclaimable() uint64 {
	s := []metrics.Sample{
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/gc/heap/live:bytes"},
		{Name: "/memory/classes/heap/free:bytes"},
	}
	metrics.Read(s)
	for _, x := range s {
		if x.Value.Kind() != metrics.KindUint64 {
			return 0
		}
	}
	objects, live, free := s[0].Value.Uint64(), s[1].Value.Uint64(), s[2].Value.Uint64()
	var garbage uint64
	if objects > live {
		garbage = objects - live
	}
	return garbage + free
}
