package main

import (
	"io/ioutil"
	"reflect"
	"testing"
)

func TestParseMov(t *testing.T) {
	cases := []struct {
		file    string
		verbose bool
		want    *Mov
	}{
		{
			file:    "testdata/ffprobe1.output",
			verbose: false,
			want: &Mov{
				FPS:         "23.976",
				TimecodeIn:  "00:00:00:00",
				TimecodeOut: "00:00:00:21",
				Duration:    "22",
				Resolution:  "1920*1080",
				Codec:       "Prores HQ",
			},
		},
		{
			file:    "testdata/ffprobe2.output",
			verbose: false,
			want: &Mov{
				FPS:         "23.976",
				TimecodeIn:  "00:00:00:00",
				TimecodeOut: "00:00:04:10",
				Duration:    "107",
				Resolution:  "1920*1080",
				Codec:       "Prores HQ",
				Colorspace:  "rec709",
			},
		},
		{
			file:    "testdata/ffprobe3.output",
			verbose: false,
			want: &Mov{
				FPS:         "23.976",
				TimecodeIn:  "12:14:20:17",
				TimecodeOut: "12:14:21:22",
				Duration:    "30",
				Resolution:  "1920*1080",
				Codec:       "Prores HQ",
			},
		},
		{
			file:    "testdata/ffprobe4.output",
			verbose: false,
			want: &Mov{
				FPS:         "23.976",
				TimecodeIn:  "00:00:00:00",
				TimecodeOut: "00:00:01:04",
				Duration:    "29",
				Resolution:  "1920*1134",
				Codec:       "Prores Standard",
				Colorspace:  "Output - Rec.709",
			},
		},
	}
	for _, c := range cases {
		info, err := ioutil.ReadFile(c.file)
		if err != nil {
			t.Fatalf("read error: %v", err)
		}
		got, err := parseMovinfo(info, c.verbose)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("got %v, want %v", got, c.want)
		}
	}
}
