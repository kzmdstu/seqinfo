package main

import (
	"fmt"
	"strconv"
)

// Timecode is timecode system that supports 24 and 30 base fps.
// See introduction of drop frame timecode system at http://andrewduncan.net/timecodes/
type Timecode struct {
	// base is base frame rate for timecode
	// ex) base frame rate of 29.976 fps is 30.
	base  int
	drop  bool
	frame int
}

// NewTimecode creates new Timecode.
func NewTimecode(code string, base int, drop bool) (*Timecode, error) {
	if base != 24 && base != 30 {
		return nil, fmt.Errorf("unknown base for timecode: %v:", base)
	}
	if base == 24 && drop {
		// 23.98, 23.978 isn't a drop timecode system.
		drop = false
	}
	if len(code) != 11 {
		return nil, fmt.Errorf("invalid timecode: %v", code)
	}
	codes := [4]int{}
	for i := 0; i < len(code); i += 3 {
		n, err := strconv.Atoi(code[i : i+2])
		if err != nil {
			return nil, fmt.Errorf("invalid timecode: %v", code)
		}
		codes[i/3] = n
	}
	h := codes[0]
	m := codes[1]
	s := codes[2]
	f := codes[3]
	frame := 3600*h*base + 60*m*base + s*base + f
	if drop {
		// assume it is base 30
		totalMinutes := 60*h + m
		frame -= 2 * (totalMinutes - totalMinutes/10)
	}
	t := &Timecode{
		base:  base,
		drop:  drop,
		frame: frame,
	}
	return t, nil
}

// Add adds frames to the Timecode.
func (t *Timecode) Add(n int) {
	t.frame += n
}

// String represents the Timecode as string.
func (t *Timecode) String() string {
	base := t.base
	frame := t.frame
	if t.drop {
		// assume it is base 30
		D := frame / 17982  // number of "full" 10 minutes chunks in drop frame system
		M := frame % 17982  // remainder frames
		d := (M - 2) / 1798 // number of 1 minute chunks those drop frames; M-2 because the first chunk will not drop frames
		frame += 18*D + 2*d // 10 minutes chunks drop 18 frames; 1 minute chunks drop 2 frames
	}
	h := frame / base / 60 / 60 % 24
	m := frame / base / 60 % 60
	s := frame / base % 60
	f := frame % base
	codes := [4]int{h, m, s, f}
	timecode := ""
	for i, c := range codes {
		if i == 1 || i == 2 {
			timecode += ":"
		}
		if i == 3 {
			if t.drop {
				timecode += ";"
			} else {
				timecode += ":"
			}
		}
		tc := strconv.Itoa(c)
		if len(tc) == 1 {
			tc = "0" + tc
		}
		timecode += tc
	}
	return timecode
}
