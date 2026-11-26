package app

import (
	"time"

	"netease-music-mygo/internal/netease"
)

// What the listener may play of a song comes with it, as its privilege,
// from when it was loaded: a membership taken out or renewed since changes
// that, and songs kept from before (the queue of the last run, a list
// opened a while ago) go on saying what was so then. Renewed, VIP songs
// stayed grey and could not be played (1.3.9). These ask NetEase again.

// privilegeDetails returns songs with their privileges now; tests replace it.
var privilegeDetails = netease.SongDetails

// recheckGap is how soon the same songs may be asked about again.
const recheckGap = 10 * time.Second

// queueRecheckGap is how often the queue is looked over as the window
// comes back to the front.
const queueRecheckGap = 10 * time.Minute

// looksUnplayable reports whether tr is a NetEase song its privilege keeps
// from playing: one a membership could let play.
func looksUnplayable(tr *netease.Track) bool {
	return tr.Source == nil && tr.ID > 0 && tr.ProgramID == 0 && tr.LocalPath == "" && !tr.Playable()
}

// recheckPlayable asks NetEase what the listener may play now of those of
// tracks that look unplayable, and puts the answers in tracks, in place,
// and in the queue; done runs then, on the main thread, if the listener is
// still the one signed in. Nothing to ask, or asked too lately, done runs
// at once.
func (a *app) recheckPlayable(tracks []netease.Track, done func()) {
	now := time.Now()
	var ids []int64
	for i := range tracks {
		tr := &tracks[i]
		if !looksUnplayable(tr) || now.Sub(a.privAsked[tr.ID]) < recheckGap {
			continue
		}
		if a.privAsked == nil {
			a.privAsked = map[int64]time.Time{}
		}
		a.privAsked[tr.ID] = now
		ids = append(ids, tr.ID)
	}
	if len(ids) == 0 || a.profile == nil {
		if done != nil {
			done()
		}
		return
	}
	gen := a.accountGen
	go func() {
		songs, _ := privilegeDetails(ids) // as many as came
		by := make(map[int64]netease.Privilege, len(songs))
		for _, s := range songs {
			if s.Privilege != nil {
				by[s.ID] = *s.Privilege
			}
		}
		a.post(func() { a.privilegesCame(gen, tracks, by, done) })
	}()
}

// privilegesCame puts the privileges NetEase answered with, by ID, in
// tracks and the queue, then runs done: for the listener who asked only.
func (a *app) privilegesCame(gen int, tracks []netease.Track, by map[int64]netease.Privilege, done func()) {
	if gen != a.accountGen {
		return
	}
	for i := range tracks {
		if pr, ok := by[tracks[i].ID]; ok && tracks[i].ProgramID == 0 && tracks[i].LocalPath == "" {
			tracks[i].Privilege = &pr
		}
	}
	if a.privKnown == nil {
		a.privKnown = map[int64]netease.Privilege{}
	}
	for id, pr := range by {
		a.privKnown[id] = pr
	}
	a.pl.SetPrivileges(by)
	if done != nil {
		done()
	}
}

// knownPrivileges puts in tracks, in place, what NetEase last said may be
// played of them, and asks it of those still looking unplayable: a list
// kept without privileges, as the listening ledger keeps its songs, would
// otherwise grey every VIP song for a member. Each song is asked once in a
// while (recheckGap), so calling it every frame asks once.
func (a *app) knownPrivileges(tracks []netease.Track) {
	if a.profile == nil {
		return
	}
	ask := false
	for i := range tracks {
		tr := &tracks[i]
		if pr, ok := a.privKnown[tr.ID]; ok && tr.ProgramID == 0 && tr.LocalPath == "" {
			tr.Privilege = &pr
		} else if looksUnplayable(tr) {
			ask = true
		}
	}
	if ask {
		a.recheckPlayable(append([]netease.Track(nil), tracks...), a.redraw)
	}
}

// recheckQueue looks over the queue's songs that look unplayable: as the
// last run's comes back, and now and then as the window does.
func (a *app) recheckQueue(force bool) {
	now := time.Now()
	if !force && now.Sub(a.queueRechecked) < queueRecheckGap {
		return
	}
	a.queueRechecked = now
	queue := a.pl.Queue()
	if len(queue) == 0 {
		return
	}
	// A copy: the queue may change before the answer comes; the player
	// takes the answers by ID.
	a.recheckPlayable(append([]netease.Track(nil), queue...), nil)
}

// playGrey is a press on a song that looks unplayable, at i of tracks: it
// may have become playable, the membership renewed, so NetEase is asked
// first, and play runs if it has; otherwise a toast says why it does not.
func (a *app) playGrey(tracks []netease.Track, i int, play func()) {
	a.recheckPlayable(tracks, func() {
		if i < len(tracks) && tracks[i].Playable() {
			play()
			return
		}
		if pr := tracks[min(i, len(tracks)-1)].Privilege; pr != nil && pr.St < 0 {
			a.say("unplayable", "这首歌暂无版权")
			return
		}
		a.say("unplayable", "这首歌需要 VIP 才能播放")
	})
}
