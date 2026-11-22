package qqmusic

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Album and artist responses must preserve version titles instead of using the base name.
func TestCatalogFullTitles(t *testing.T) {
	old := client
	defer func() { client = old }()
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		row := `{"songInfo":{"mid":"version","name":"座位","title":"座位 (架子鼓版)","index_album":2,"interval":224}}`
		body := `{"code":0,"info":{"code":0,"data":{"basicInfo":{"albumName":"座位"}}},"songs":{"code":0,"data":{"totalNum":2,"songList":[` + row + `,{"songInfo":{"mid":"base","name":"座位","index_album":1}}]}},"req":{"code":0,"data":{"totalNum":1,"songList":[` + row + `]}}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	al, err := AlbumDetail(context.Background(), "album")
	if err != nil {
		t.Fatal(err)
	}
	if len(al.Songs) != 2 || al.Songs[0].MID != "base" || al.Songs[0].Name != "座位" || al.Songs[1].Name != "座位 (架子鼓版)" {
		t.Fatalf("wrong titles/order: %+v", al.Songs)
	}
	ar, err := ArtistSongs(context.Background(), "artist", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ar.Songs) != 1 || ar.Songs[0].Name != "座位 (架子鼓版)" {
		t.Fatalf("lost artist version: %+v", ar.Songs)
	}
}
