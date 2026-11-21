package netease

import "testing"

func TestPlayableFollowsPrivilege(t *testing.T) {
	tracks := []Track{{ID: 1, Fee: 1}, {ID: 2, Fee: 8}, {ID: 3, Fee: 8}, {ID: 4, Fee: 1}}
	if tracks[0].Playable() || !tracks[1].Playable() {
		t.Fatal("without privileges, VIP songs do not play and free ones do")
	}
	withPrivileges(tracks, []Privilege{
		{ID: 1, Pl: 320000},           // VIP song, the listener subscribes
		{ID: 2, Pl: 0},                // nothing to play
		{ID: 3, St: -200, Pl: 128000}, // taken down
	})
	want := []bool{true, false, false, false}
	for i, tr := range tracks {
		if tr.Playable() != want[i] {
			t.Errorf("track %d: playable %v", tr.ID, tr.Playable())
		}
	}
}

// RequiresVIP identifies catalog and privilege-marked tracks blocked for free accounts.
func TestRequiresVIP(t *testing.T) {
	tests := []struct {
		name  string
		track Track
		want  bool
	}{
		{"vip fee", Track{Fee: 1}, true},
		{"paid album", Track{Fee: 4}, true},
		{"unavailable privilege", Track{Privilege: &Privilege{St: 0, Pl: 0}}, true},
		{"taken down", Track{Privilege: &Privilege{St: -1, Pl: 0}}, false},
		{"playable", Track{Fee: 0, Privilege: &Privilege{St: 0, Pl: 320000}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.track.RequiresVIP(); got != tt.want {
				t.Fatalf("RequiresVIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

// No-copyright tracks are eligible for another source, while VIP tracks are not.
func TestNoCopyright(t *testing.T) {
	noRights := Track{Privilege: &Privilege{St: -200}}
	if !noRights.NoCopyright() || noRights.RequiresVIP() {
		t.Fatalf("no-copyright track classification is wrong: %+v", noRights.Privilege)
	}
	vip := Track{Fee: 1, Privilege: &Privilege{St: 0}}
	if vip.NoCopyright() || !vip.RequiresVIP() {
		t.Fatalf("VIP track classification is wrong: %+v", vip.Privilege)
	}
}
