package player

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// AIFF is the Mac's WAV: the same PCM, its samples big-endian, its chunks
// in a "FORM". AIFF-C adds a compression type, of which "NONE" and "sowt"
// (little-endian) are PCM still and "fl32"/"fl64" floats; others are
// compressed, and not read (#65). It plays as a wavStream does.

// readAIFFHeader reads the chunks of the AIFF at off, from "FORM" to the
// start of its sound in "SSND".
func readAIFFHeader(dl *download, off int64) (wavFormat, error) {
	be := binary.BigEndian
	if !dl.waitAt(off, 12) {
		return wavFormat{}, io.ErrUnexpectedEOF
	}
	head := dl.peek(off, 12)
	kind := string(head[8:12])
	if string(head[:4]) != "FORM" || kind != "AIFF" && kind != "AIFC" {
		return wavFormat{}, errors.New("aiff: not an AIFF file")
	}
	var f wavFormat
	var haveComm bool
	f.bigEndian = true
	for at := off + 12; ; {
		if !dl.waitAt(at, 8) {
			return wavFormat{}, errors.New("aiff: no sound chunk")
		}
		c := dl.peek(at, 8)
		id, n := string(c[:4]), int64(be.Uint32(c[4:8]))
		body := at + 8
		switch id {
		case "COMM":
			if n < 18 || n > 1<<10 || !dl.waitAt(body, n) {
				return wavFormat{}, errors.New("aiff: bad common chunk")
			}
			b := dl.peek(body, int(n))
			f.channels = int(int16(be.Uint16(b[0:2])))
			f.valid = int(int16(be.Uint16(b[6:8])))
			f.rate = int(math.Round(extended80(b[8:18])))
			f.bits = (f.valid + 7) / 8 * 8
			if kind == "AIFC" && len(b) >= 22 {
				switch string(b[18:22]) {
				case "NONE", "twos":
				case "sowt":
					f.bigEndian = false
				case "fl32", "FL32":
					f.float, f.bits, f.valid = true, 32, 32
				case "fl64", "FL64":
					f.float, f.bits, f.valid = true, 64, 64
				default:
					return wavFormat{}, errors.New("aiff: compressed samples")
				}
			}
			if f.channels <= 0 || f.rate <= 0 || f.bits < 8 || f.bits > 32 && !f.float {
				return wavFormat{}, errors.New("aiff: unexpected format")
			}
			f.align = f.channels * f.bits / 8
			f.signed8 = true
			haveComm = true
		case "SSND":
			if !haveComm || !dl.waitAt(body, 8) {
				return wavFormat{}, errors.New("aiff: sound before its format")
			}
			skip := int64(be.Uint32(dl.peek(body, 4)))
			start := body + 8 + skip
			n -= 8 + skip
			if size := dl.totalSize(); size > 0 && (n <= 0 || start+n > size) {
				n = size - start
			}
			f.start, f.length = start, n
			return f, nil
		}
		at = body + n + n&1
	}
}

func openAIFFStream(dl *download) (*wavStream, error) {
	f, err := readAIFFHeader(dl, id3Size(dl))
	if err != nil {
		return nil, err
	}
	s := &wavStream{dl: dl, wavFormat: f, frames: f.length / int64(f.align), drop: dl.Close}
	s.spec = dl.spectrum(f.rate, s.frames)
	s.seek(0)
	return s, nil
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
