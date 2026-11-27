package app

import (
	"strings"

	"netease-music-mygo/internal/netease"
)

// expandedArtists normalizes provider credit fields that put several
// performers in one name. A composite field has no reliable per-artist ID,
// so its parts deliberately resolve by song metadata instead of inheriting
// one ID for every performer.
func expandedArtists(artists []netease.Artist) []netease.Artist {
	var out []netease.Artist
	for _, artist := range artists {
		// Entries are normally already structured artists. Split only the
		// comma forms catalogs sometimes leave inside one entry; '/' and '&'
		// can be part of a real artist name (for example AC/DC).
		parts := strings.FieldsFunc(artist.Name, func(r rune) bool {
			return r == ',' || r == '，' || r == '、' || r == '､'
		})
		if len(parts) <= 1 {
			if len(parts) == 1 {
				artist.Name = parts[0]
			}
			out = append(out, artist)
			continue
		}
		for _, name := range parts {
			out = append(out, netease.Artist{Name: name})
		}
	}
	return out
}

func trackArtistNames(track netease.Track) []string {
	artists := expandedArtists(track.Artists)
	names := make([]string, 0, len(artists))
	for _, artist := range artists {
		names = append(names, artist.Name)
	}
	return names
}
