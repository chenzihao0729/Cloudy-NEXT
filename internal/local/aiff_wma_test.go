package local

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"
)

func ext80(rate float64) []byte {
	e := int(math.Floor(math.Log2(rate)))
	b := make([]byte, 10)
	binary.BigEndian.PutUint16(b, uint16(e+16383))
	binary.BigEndian.PutUint64(b[2:], uint64(rate*math.Pow(2, float64(63-e))))
	return b
}

func u16(s string) []byte {
	var b bytes.Buffer
	for _, r := range utf16.Encode([]rune(s + "\x00")) {
		binary.Write(&b, binary.LittleEndian, r)
	}
	return b.Bytes()
}

// An AIFF and a WMA in the listener's folder list with their length and
// what their tags say (#65).
func TestReadAIFFAndWMA(t *testing.T) {
	dir := t.TempDir()
	be, le := binary.BigEndian, binary.LittleEndian

	// Two and a half seconds at 44.1 kHz, named in its NAME and AUTH.
	var comm bytes.Buffer
	binary.Write(&comm, be, int16(2))
	binary.Write(&comm, be, uint32(110250))
	binary.Write(&comm, be, int16(16))
	comm.Write(ext80(44100))
	var body bytes.Buffer
	body.WriteString("AIFF")
	for _, c := range []struct {
		id string
		b  []byte
	}{{"COMM", comm.Bytes()}, {"NAME", []byte("晴天")}, {"AUTH", []byte("周杰伦")}, {"SSND", make([]byte, 8)}} {
		body.WriteString(c.id)
		binary.Write(&body, be, uint32(len(c.b)))
		body.Write(c.b)
		if len(c.b)%2 == 1 {
			body.WriteByte(0)
		}
	}
	var aiff bytes.Buffer
	aiff.WriteString("FORM")
	binary.Write(&aiff, be, uint32(body.Len()))
	aiff.Write(body.Bytes())
	aiffPath := filepath.Join(dir, "01 x.aiff")
	os.WriteFile(aiffPath, aiff.Bytes(), 0o644)

	// A WMA's header: its file properties (3 minutes, a preroll of 3 s)
	// and its content descriptions.
	object := func(guid []byte, b []byte) []byte {
		var o bytes.Buffer
		o.Write(guid)
		binary.Write(&o, le, uint64(24+len(b)))
		o.Write(b)
		return o.Bytes()
	}
	file := make([]byte, 80)
	binary.LittleEndian.PutUint64(file[40:], uint64((183*time.Second)/100))
	binary.LittleEndian.PutUint64(file[56:], 3000)
	title, artist := u16("稻香"), u16("周杰伦")
	var content bytes.Buffer
	for _, n := range []int{len(title), len(artist), 0, 0, 0} {
		binary.Write(&content, le, uint16(n))
	}
	content.Write(title)
	content.Write(artist)
	var ext bytes.Buffer
	binary.Write(&ext, le, uint16(2))
	attr := func(name string, kind uint16, v []byte) {
		n := u16(name)
		binary.Write(&ext, le, uint16(len(n)))
		ext.Write(n)
		binary.Write(&ext, le, kind)
		binary.Write(&ext, le, uint16(len(v)))
		ext.Write(v)
	}
	attr("WM/AlbumTitle", 0, u16("魔杰座"))
	track := make([]byte, 4)
	binary.LittleEndian.PutUint32(track, 7)
	attr("WM/TrackNumber", 3, track)
	objs := append(append(object(asfFile, file), object(asfContent, content.Bytes())...), object(asfExtContent, ext.Bytes())...)
	var wma bytes.Buffer
	wma.Write(asfHeader)
	binary.Write(&wma, le, uint64(30+len(objs)))
	binary.Write(&wma, le, uint32(3))
	wma.Write([]byte{1, 2})
	wma.Write(objs)
	wmaPath := filepath.Join(dir, "y.wma")
	os.WriteFile(wmaPath, wma.Bytes(), 0o644)

	for _, c := range []struct {
		path, title, artist, album string
		d                          time.Duration
		track                      int
	}{
		{aiffPath, "晴天", "周杰伦", "", 2500 * time.Millisecond, 0},
		{wmaPath, "稻香", "周杰伦", "魔杰座", 180 * time.Second, 7},
	} {
		if !Playable(filepath.Ext(c.path)) {
			t.Fatalf("%s not taken", c.path)
		}
		s, err := Read(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if s.Title != c.title || s.Artist != c.artist || s.Album != c.album || s.Track != c.track {
			t.Errorf("%s: %q by %q on %q, track %d", filepath.Base(c.path), s.Title, s.Artist, s.Album, s.Track)
		}
		if d := s.Duration - c.d; d < -time.Millisecond || d > time.Millisecond {
			t.Errorf("%s: %v long", filepath.Base(c.path), s.Duration)
		}
	}
}
