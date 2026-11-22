package sourcecatalog

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Migu's missing search duration is filled from the exact copyright ID's detail.
func TestMiguSearchFillsDuration(t *testing.T) {
	old := client
	defer func() { client = old }()
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"code":"000000","songResultData":{"result":[{"copyrightId":"jay-id","name":"红颜如霜"}]}}`
		if strings.Contains(r.URL.Path, "resourceinfo") {
			if r.URL.Query().Get("copyrightId") != "jay-id" {
				t.Error("detail used wrong recording")
			}
			body = `{"resource":[{"length":"00:04:17"}]}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	songs, err := searchMiguApp(context.Background(), "红颜如霜", 20)
	if err != nil || len(songs) != 1 || songs[0].Duration != 257*time.Second {
		t.Fatalf("%+v %v", songs, err)
	}
}
