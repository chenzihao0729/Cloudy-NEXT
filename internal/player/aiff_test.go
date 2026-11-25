package player

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// putExtended80 writes rate as AIFF's 80-bit extended float.
func putExtended80(rate float64) []byte {
	e := int(math.Floor(math.Log2(rate)))
	b := make([]byte, 10)
	binary.BigEndian.PutUint16(b, uint16(e+16383))
	binary.BigEndian.PutUint64(b[2:], uint64(rate*math.Pow(2, float64(63-e))))
	return b
}

// aiffFile is a second of a 1 kHz tone at amplitude 0.5 in an AIFF, or an
// AIFF-C compressed as kind says: "NONE", "sowt" (little-endian) or
// "fl32".
func aiffFile(channels, bits int, kind string, rate int) []byte {
	be := binary.BigEndian
	var order binary.ByteOrder = be
	if kind == "sowt" {
		order = binary.LittleEndian
	}
	var data bytes.Buffer
	for n := 0; n < rate; n++ {
		x := 0.5 * math.Sin(2*math.Pi*1000*float64(n)/float64(rate))
		for c := 0; c < channels; c++ {
			switch {
			case kind == "fl32":
				binary.Write(&data, be, float32(x))
			case bits == 8:
				data.WriteByte(byte(int8(math.Round(x * 127))))
			case bits == 16:
				binary.Write(&data, order, int16(math.Round(x*32767)))
			case bits == 24:
				v := int32(math.Round(x * (1<<23 - 1)))
				data.Write([]byte{byte(v >> 16), byte(v >> 8), byte(v)})
			}
		}
	}
	var comm bytes.Buffer
	binary.Write(&comm, be, int16(channels))
	binary.Write(&comm, be, uint32(rate))
	binary.Write(&comm, be, int16(bits))
	comm.Write(putExtended80(float64(rate)))
	form := "AIFF"
	if kind != "" {
		form = "AIFC"
		comm.WriteString(kind)
		comm.Write([]byte{0}) // an empty name, padded
		comm.Write([]byte{0})
	}
	var body bytes.Buffer
	body.WriteString(form)
	chunk := func(id string, b []byte) {
		body.WriteString(id)
		binary.Write(&body, be, uint32(len(b)))
		body.Write(b)
		if len(b)%2 == 1 {
			body.WriteByte(0)
		}
	}
	chunk("COMM", comm.Bytes())
	chunk("NAME", []byte("tone"))
	ssnd := append(make([]byte, 8), data.Bytes()...)
	chunk("SSND", ssnd)
	var f bytes.Buffer
	f.WriteString("FORM")
	binary.Write(&f, be, uint32(body.Len()))
	f.Write(body.Bytes())
	return f.Bytes()
}

// An AIFF plays, as a WAV does (#65): each kind of sample the tone at its
// pitch and loudness, a second of it, from the start and from half in.
func TestAIFFDecodes(t *testing.T) {
	cases := map[string]struct {
		channels, bits int
		kind           string
	}{
		"16-bit":            {2, 16, ""},
		"8-bit mono":        {1, 8, ""},
		"24-bit":            {2, 24, ""},
		"AIFC sowt":         {2, 16, "sowt"},
		"AIFC float":        {2, 32, "fl32"},
		"AIFC uncompressed": {2, 16, "NONE"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := openWAVFile(t, aiffFile(c.channels, c.bits, c.kind, 48000))
			if s.SampleRate() != 48000 || s.Len() != 48000 {
				t.Fatalf("rate %d, length %d", s.SampleRate(), s.Len())
			}
			frames, crossings, power := decodeTone(t, s)
			if frames != 48000 || crossings < 1990 || crossings > 2010 {
				t.Errorf("%d frames, %d crossings", frames, crossings)
			}
			if math.Abs(power-0.125) > 0.005 {
				t.Errorf("power %v", power)
			}
			if err := s.SetPosition(24000); err != nil {
				t.Fatal(err)
			}
			if frames, crossings, _ := decodeTone(t, s); frames != 24000 || crossings < 990 || crossings > 1010 {
				t.Errorf("after seeking: %d frames, %d crossings", frames, crossings)
			}
		})
	}
	if f := otherFormat(aiffFile(2, 16, "", 8000)[:12]); f != "AIFF" {
		t.Errorf("taken for %q", f)
	}
}
