package rss

import (
	"testing"

	"gopkg.in/h2non/gock.v1"
)

func TestCheckBoardExist(t *testing.T) {
	defer gock.Off()

	// Mock existing board
	gock.New("https://www.ptt.cc").
		Get("/atom/movie.xml").
		Reply(200).
		BodyString(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>movie</title>
  <entry><title>Test</title></entry>
</feed>`)

	// Mock non-existing board
	gock.New("https://www.ptt.cc").
		Get("/atom/movies.xml").
		Reply(404)

	type args struct {
		board string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"exist", args{"movie"}, true},
		{"not exist", args{"movies"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckBoardExist(tt.args.board); got != tt.want {
				t.Errorf("CheckBoardExist() = %v, want %v", got, tt.want)
			}
		})
	}
}
