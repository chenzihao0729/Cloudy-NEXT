package local

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// AIFF and WMA, which the player plays as of 1.3.10 (#65): their length,
// and what their tags say.

// readAIFF reads an AIFF's length from its "COMM" chunk, and its tags from
// an "ID3 " chunk, or else its "NAME" and "AUTH".
func readAIFF(r io.ReaderAt, size int64, s *Song) {
	be := binary.BigEndian
	var h [12]byte
	if _, err := r.ReadAt(h[:], 0); err != nil || string(h[:4]) != "FORM" {
		return
	}
	if kind := string(h[8:12]); kind != "AIFF" && kind != "AIFC" {
		return
	}
	var name, author string
	for off := int64(12); off+8 <= size; {
		var c [8]byte
		if _, err := r.ReadAt(c[:], off); err != nil {
			break
		}
		id, n := string(c[:4]), int64(be.Uint32(c[4:8]))
		body := off + 8
		switch id {
		case "COMM":
			var b [18]byte
			if _, err := r.ReadAt(b[:], body); err == nil {
				frames := float64(be.Uint32(b[2:6]))
				if rate := extended80(b[8:18]); rate > 0 {
					s.Duration = time.Duration(frames / rate * float64(time.Second))
				}
			}
		case "ID3 ", "id3 ":
			if n > 10 && n < 64<<20 && body+n <= size {
				b := make([]byte, n)
				if _, err := r.ReadAt(b, body); err == nil {
					readID3v2(b, s)
				}
			}
		case "NAME", "AUTH":
			if n > 0 && n < 1<<10 && body+n <= size {
				b := make([]byte, n)
				if _, err := r.ReadAt(b, body); err == nil {
					if id == "NAME" {
						name = riffText(b)
					} else {
						author = riffText(b)
					}
				}
			}
		}
		off = body + n + n&1
	}
	s.Title = first(s.Title, name)
	s.Artist = first(s.Artist, author)
}

// extended80 reads an 80-bit IEEE 754 extended float, big-endian, as AIFF
// gives its sample rate.
func extended80(b []byte) float64 {
	exp := int(binary.BigEndian.Uint16(b[0:2]))
	mant := binary.BigEndian.Uint64(b[2:10])
	if exp == 0 && mant == 0 {
		return 0
	}
	sign := 1.0
	if exp&0x8000 != 0 {
		sign, exp = -1, exp&0x7fff
	}
	return sign * math.Ldexp(float64(mant), exp-16383-63)
}

// The ASF objects a WMA's header holds that say what it is, by their
// GUIDs, as they are written.
var (
	asfHeader      = []byte{0x30, 0x26, 0xb2, 0x75, 0x8e, 0x66, 0xcf, 0x11, 0xa6, 0xd9, 0x00, 0xaa, 0x00, 0x62, 0xce, 0x6c}
	asfFile        = []byte{0xa1, 0xdc, 0xab, 0x8c, 0x47, 0xa9, 0xcf, 0x11, 0x8e, 0xe4, 0x00, 0xc0, 0x0c, 0x20, 0x53, 0x65}
	asfContent     = []byte{0x33, 0x26, 0xb2, 0x75, 0x8e, 0x66, 0xcf, 0x11, 0xa6, 0xd9, 0x00, 0xaa, 0x00, 0x62, 0xce, 0x6c}
	asfExtContent  = []byte{0x40, 0xa4, 0xd0, 0xd2, 0x07, 0xe3, 0xd2, 0x11, 0x97, 0xf0, 0x00, 0xa0, 0xc9, 0x5e, 0xa8, 0x50}
	asfHeaderLimit = int64(16 << 20) // more is pictures, which these don't read
)

// readWMA reads a WMA's length from its file properties, and its title,
// artist, album and track from its content descriptions.
func readWMA(r io.ReaderAt, size int64, s *Song) {
	le := binary.LittleEndian
	var h [30]byte
	if _, err := r.ReadAt(h[:], 0); err != nil || !bytes.Equal(h[:16], asfHeader) {
		return
	}
	end := min(int64(le.Uint64(h[16:24])), size, asfHeaderLimit)
	for off := int64(30); off+24 <= end; {
		var c [24]byte
		if _, err := r.ReadAt(c[:], off); err != nil {
			break
		}
		n := int64(le.Uint64(c[16:24]))
		if n < 24 || off+n > end {
			break
		}
		body := make([]byte, n-24)
		if _, err := r.ReadAt(body, off+24); err != nil {
			break
		}
		switch {
		case bytes.Equal(c[:16], asfFile) && len(body) >= 64:
			// The play duration in 100 ns, and the preroll, in ms, it
			// counts.
			play := int64(le.Uint64(body[40:48]))
			preroll := int64(le.Uint64(body[56:64]))
			if d := time.Duration(play)*100 - time.Duration(preroll)*time.Millisecond; d > 0 {
				s.Duration = d
			}
		case bytes.Equal(c[:16], asfContent) && len(body) >= 10:
			lens := [5]int{}
			for i := range lens {
				lens[i] = int(le.Uint16(body[2*i:]))
			}
			at := 10
			text := func(n int) string {
				if at+n > len(body) {
					return ""
				}
				v := utf16le(body[at : at+n])
				at += n
				return v
			}
			s.Title = first(s.Title, text(lens[0]))
			s.Artist = first(s.Artist, text(lens[1]))
		case bytes.Equal(c[:16], asfExtContent) && len(body) >= 2:
			readASFExtended(body, s)
		}
		off += n
	}
}

// readASFExtended reads the album, the album's artist and the track from
// an extended content description's attributes.
func readASFExtended(b []byte, s *Song) {
	le := binary.LittleEndian
	count := int(le.Uint16(b))
	at := 2
	for range count {
		if at+2 > len(b) {
			return
		}
		nameLen := int(le.Uint16(b[at:]))
		at += 2
		if at+nameLen+4 > len(b) {
			return
		}
		name := utf16le(b[at : at+nameLen])
		at += nameLen
		kind, valueLen := le.Uint16(b[at:]), int(le.Uint16(b[at+2:]))
		at += 4
		if at+valueLen > len(b) {
			return
		}
		v := b[at : at+valueLen]
		at += valueLen
		str := ""
		switch kind {
		case 0: // a string
			str = utf16le(v)
		case 3: // a DWORD
			if len(v) >= 4 {
				str = strconv.Itoa(int(le.Uint32(v)))
			}
		}
		switch name {
		case "WM/AlbumTitle":
			s.Album = first(s.Album, str)
		case "WM/AlbumArtist":
			s.Artist = first(s.Artist, str)
		case "WM/TrackNumber", "WM/Track":
			if s.Track == 0 {
				s.Track = number(str)
			}
		}
	}
}

// utf16le decodes b, UTF-16 little-endian, to the first NUL.
func utf16le(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	v := string(utf16.Decode(u))
	if i := strings.IndexByte(v, 0); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}
