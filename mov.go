package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Mov is a mov info that will be used by seqinfo.
type Mov struct {
	File        string
	TimecodeIn  string
	TimecodeOut string
	Duration    string
	FPS         string
	Resolution  string
	Codec       string
	Colorspace  string
}

// ffOutput is a mov info got by ffprobe.
type ffOutput struct {
	Streams []ffStream `json:"streams"`
	Format  ffFormat   `json:"format"`
}

// ffStream is a mov stream info got by ffprobe.
// There is video streams and audio streams, but we only need video info.
type ffStream struct {
	NbFrames   string       `json:"nb_frames"`
	RFrameRate string       `json:"r_frame_rate"`
	CodecName  string       `json:"codec_name"`
	Profile    string       `json:"profile"`
	Width      int          `json:"width"`
	Height     int          `json:"height"`
	Tags       ffStreamTags `json:"tags"`
}

// ffStreamTags is a mov stream tag info got by ffprobe.
type ffStreamTags struct {
	Timecode string `json:"timecode"`
}

// ffFormat is a mov format info got by ffprobe.
type ffFormat struct {
	Tags ffFormatTags `json:"tags"`
}

// ffFormatTags is a mov format tag info got by ffprobe.
type ffFormatTags struct {
	FoundryColorspace string `json:"uk.co.thefoundry.Colorspace"`
}

// parseMov got a mov file path and parses its info.
// If verbose is true, it will put error message in the field when there is a missing info,
// instead of leaving it empty.
// It will only return an error, when there is a crucial error occurred while running ffprobe.
func parseMov(file string, verbose bool) (*Mov, error) {
	c := exec.Command("ffprobe", "-v", "quiet", "-show_format", "-show_streams", "-select_streams", "v:0", "-of", "json", file)
	b, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute: %s", b)
	}
	mov, err := parseMovinfo(b, verbose)
	if err != nil {
		return nil, err
	}
	mov.File = file
	return mov, nil
}

func parseMovinfo(info []byte, verbose bool) (*Mov, error) {
	ff := ffOutput{}
	err := json.Unmarshal(info, &ff)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %v", err)
	}
	if len(ff.Streams) > 1 {
		return nil, fmt.Errorf("too many video streams")
	}
	if len(ff.Streams) == 0 {
		return nil, fmt.Errorf("no video streams")
	}
	video := ff.Streams[0]
	mov := &Mov{}
	if err != nil && verbose {
		mov.TimecodeIn = err.Error()
	}
	mov.TimecodeIn, err = func() (string, error) {
		if video.Tags.Timecode == "" {
			// timecode may not exists
			return "", nil
		}
		return video.Tags.Timecode, nil
	}()
	if err != nil && verbose {
		mov.TimecodeIn = err.Error()
	}
	mov.TimecodeOut, err = func() (string, error) {
		if video.Tags.Timecode == "" {
			// timecode may not exists
			return "", nil
		}
		if video.RFrameRate == "" {
			return "", fmt.Errorf("missing r_frame_rate information")
		}
		if video.NbFrames == "" {
			return "", fmt.Errorf("missing nb_frames information")
		}
		var base int
		var drop bool
		switch video.RFrameRate {
		case "24/1":
			base = 24
			drop = false
		case "24000/1001":
			base = 24
			drop = true
		case "30/1":
			base = 30
			drop = false
		default:
			return "", fmt.Errorf("unknown r_frame_rate: %v", video.RFrameRate)
		}
		tc, err := NewTimecode(video.Tags.Timecode, base, drop)
		if err != nil {
			return "", err
		}
		frames, err := strconv.Atoi(video.NbFrames)
		if err != nil {
			return "", err
		}
		tc.Add(frames - 1)
		return tc.String(), nil
	}()
	if err != nil && verbose {
		mov.TimecodeOut = err.Error()
	}
	mov.Duration, err = func() (string, error) {
		if video.NbFrames == "" {
			return "", fmt.Errorf("missing nb_frames information")
		}
		return video.NbFrames, nil
	}()
	if err != nil && verbose {
		mov.Duration = err.Error()
	}
	mov.FPS, err = func() (string, error) {
		if video.RFrameRate == "" {
			return "", fmt.Errorf("missing r_frame_rate information")
		}
		var fps string
		switch video.RFrameRate {
		case "24/1":
			fps = "24"
		case "24000/1001":
			fps = "23.976"
		case "30/1":
			fps = "30"
		default:
			return "", fmt.Errorf("unknown r_frame_rate: %v", video.RFrameRate)
		}
		return fps, nil
	}()
	if err != nil && verbose {
		mov.FPS = err.Error()
	}
	mov.Resolution, err = func() (string, error) {
		if video.Width == 0 {
			return "", fmt.Errorf("missing width information")
		}
		if video.Height == 0 {
			return "", fmt.Errorf("missing height information")
		}
		return strconv.Itoa(video.Width) + "*" + strconv.Itoa(video.Height), nil
	}()
	if err != nil && verbose {
		mov.Resolution = err.Error()
	}
	mov.Codec, err = func() (string, error) {
		if video.CodecName == "" {
			return "", fmt.Errorf("missing codec_name information")
		}
		if video.Profile == "" {
			return "", fmt.Errorf("missing codec_profile information")
		}
		codec := strings.Title(strings.ToLower(video.CodecName)) + " " + video.Profile
		return codec, nil
	}()
	if err != nil && verbose {
		mov.Codec = err.Error()
	}
	mov.Colorspace, err = func() (string, error) {
		tags := ff.Format.Tags
		if tags.FoundryColorspace != "" {
			return tags.FoundryColorspace, nil
		}
		return "", nil
	}()
	if err != nil && verbose {
		mov.Colorspace = err.Error()
	}
	return mov, nil
}
