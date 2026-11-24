package player

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/mewkiz/flac"
)

// Decoding a FLAC made new sample buffers for every frame, some 0.35 MB a
// second of a CD-quality song, all of it garbage, which the listener saw
// as memory climbing steadily while a song played (1.3.7-test.5). Each
// frame now decodes into the buffers of the one before: the samples must
// still be the library's, to the last one, and a second of the song must
// make next to nothing.
func TestFLACDecodesIntoTheFrameBefore(t *testing.T) {
	// TotalAlloc counts the whole process. Other playback tests leave
	// workers finishing in the background, so measure in a fresh process
	// rather than charging their allocations to the FLAC decoder.
	const isolated = "CLOUDY_TEST_FLAC_ALLOC"
	if os.Getenv(isolated) != "1" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(exe, "-test.run=^TestFLACDecodesIntoTheFrameBefore$", "-test.count=1", "-test.timeout=1m")
		cmd.Env = append(os.Environ(), isolated+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated FLAC check: %v\n%s", err, out)
		}
		return
	}
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	// What the library itself decodes, frame by frame, made anew.
	ref, err := flac.New(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var want []float32
	for {
		f, err := ref.ParseNext()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		scale := float32(1 / math.Exp2(float64(f.BitsPerSample-1)))
		left, right := f.Subframes[0].Samples, f.Subframes[0].Samples
		if len(f.Subframes) > 1 {
			right = f.Subframes[1].Samples
		}
		for i := range left {
			want = append(want, float32(left[i])*scale, float32(right[i])*scale)
		}
	}

	s, err := openFLACStream(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	buf := make([]float32, 4096)
	got := make([]float32, 0, len(want)+len(buf))
	// The first frames make the buffers the rest decode into.
	for len(got) < 8192 {
		n, err := s.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			t.Fatal(err)
		}
	}
	from := len(got)
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	for {
		n, err := s.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			break
		}
	}
	runtime.ReadMemStats(&m1)
	if len(got) != len(want) {
		t.Fatalf("decoded %d samples, the library %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d is %v, the library's %v: a frame kept another's samples", i, got[i], want[i])
		}
	}
	sec := float64(len(got)-from) / 2 / float64(s.SampleRate())
	made := float64(m1.TotalAlloc - m0.TotalAlloc)
	if perSec := made / sec; perSec > 40<<10 {
		t.Fatalf("decoding made %.0f KB a second of audio, want next to nothing", perSec/1024)
	}
}
