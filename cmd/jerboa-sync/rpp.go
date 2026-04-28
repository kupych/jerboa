package main

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type RPPItem struct {
	TrackName string
	FilePath  string // relative path as stored in .rpp
	OffsetMS  int64
	Gain      float64 // linear gain from VOLPAN (1.0 = unity); 0 means not set
}

// RPPTrack groups everything needed to bake a single Reaper track:
// stable GUID, display name, verbatim <TRACK> block bytes (input to render-hash),
// and the audio items that live on it.
type RPPTrack struct {
	GUID       string
	Name       string
	BlockBytes []byte
	Items      []RPPItem
}

// ParseRPP parses a Reaper project file and returns all audio items with their
// track names, file paths, and timeline positions in milliseconds.
func ParseRPP(rppPath string) ([]RPPItem, error) {
	f, err := os.Open(rppPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	baseDir := filepath.Dir(rppPath)

	type frame struct {
		tag string
	}

	var stack []frame
	var items []RPPItem

	currentTrackName := ""
	currentTrackVol := 1.0 // VOLPAN from the enclosing TRACK block
	inItem := false
	inSource := false
	itemOffset := 0.0
	itemVol := 0.0 // VOLPAN from the ITEM block (0 = not set)
	sourceFile := ""

	flush := func() {
		if inItem && sourceFile != "" {
			abs := sourceFile
			if !filepath.IsAbs(sourceFile) {
				abs = filepath.Join(baseDir, sourceFile)
			}
			gain := currentTrackVol
			if itemVol > 0 {
				gain = currentTrackVol * itemVol
			}
			items = append(items, RPPItem{
				TrackName: currentTrackName,
				FilePath:  abs,
				OffsetMS:  int64(math.Round(itemOffset * 1000)),
				Gain:      gain,
			})
		}
		inItem = false
		inSource = false
		sourceFile = ""
		itemOffset = 0
		itemVol = 0
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "<") {
			tag := strings.Fields(strings.TrimPrefix(line, "<"))[0]
			stack = append(stack, frame{tag: tag})
			switch tag {
			case "TRACK":
				currentTrackVol = 1.0 // reset to unity for each new track
			case "ITEM":
				flush()
				inItem = true
			case "SOURCE":
				if inItem {
					inSource = true
				}
			}
			continue
		}

		if line == ">" {
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.tag == "ITEM" {
					flush()
				}
				if top.tag == "SOURCE" {
					inSource = false
				}
			}
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		key := parts[0]
		val := strings.TrimSpace(parts[1])

		switch {
		case key == "NAME" && len(stack) > 0 && stack[len(stack)-1].tag == "TRACK":
			currentTrackName = unquote(val)
		case key == "VOLPAN" && len(stack) > 0 && stack[len(stack)-1].tag == "TRACK":
			// VOLPAN <vol> <pan> ...  — capture track fader volume
			if v, err := strconv.ParseFloat(strings.Fields(val)[0], 64); err == nil {
				currentTrackVol = v
			}
		case key == "VOLPAN" && inItem:
			// Per-item clip gain
			if v, err := strconv.ParseFloat(strings.Fields(val)[0], 64); err == nil {
				itemVol = v
			}
		case key == "POSITION" && inItem:
			itemOffset, _ = strconv.ParseFloat(val, 64)
		case key == "FILE" && inSource:
			sourceFile = unquote(val)
			// Reaper uses backslashes on Windows; normalise
			sourceFile = filepath.FromSlash(strings.ReplaceAll(sourceFile, "\\", "/"))
		}
	}
	flush()

	return items, scanner.Err()
}

// ParseRPPTracks returns one RPPTrack per top-level <TRACK> block, capturing
// the raw block bytes so callers can hash them as part of a render-hash.
func ParseRPPTracks(rppPath string) ([]RPPTrack, error) {
	f, err := os.Open(rppPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	baseDir := filepath.Dir(rppPath)

	var tracks []RPPTrack
	var depth int
	var capturing bool
	var blockDepth int // depth at which the current TRACK block opened
	var buf strings.Builder

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)

		isOpen := strings.HasPrefix(trimmed, "<")
		isClose := trimmed == ">"

		if isOpen {
			tag := strings.Fields(strings.TrimPrefix(trimmed, "<"))[0]
			if !capturing && depth == 1 && tag == "TRACK" {
				capturing = true
				blockDepth = depth
				buf.Reset()
			}
			depth++
		}

		if capturing {
			buf.WriteString(raw)
			buf.WriteByte('\n')
		}

		if isClose {
			depth--
			if capturing && depth == blockDepth {
				blob := buf.String()
				t := parseTrackBlock(blob, baseDir)
				t.BlockBytes = []byte(blob)
				tracks = append(tracks, t)
				capturing = false
				buf.Reset()
			}
		}
	}
	return tracks, scanner.Err()
}

// parseTrackBlock parses a single captured <TRACK ... > block (as text) and
// extracts GUID, name, and audio items.
func parseTrackBlock(block, baseDir string) RPPTrack {
	var t RPPTrack
	trackVol := 1.0

	type frame struct{ tag string }
	var stack []frame

	inItem := false
	inSource := false
	itemOffset := 0.0
	itemVol := 0.0
	sourceFile := ""

	flush := func() {
		if inItem && sourceFile != "" {
			abs := sourceFile
			if !filepath.IsAbs(sourceFile) {
				abs = filepath.Join(baseDir, sourceFile)
			}
			gain := trackVol
			if itemVol > 0 {
				gain = trackVol * itemVol
			}
			t.Items = append(t.Items, RPPItem{
				TrackName: t.Name,
				FilePath:  abs,
				OffsetMS:  int64(math.Round(itemOffset * 1000)),
				Gain:      gain,
			})
		}
		inItem = false
		inSource = false
		sourceFile = ""
		itemOffset = 0
		itemVol = 0
	}

	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "<") {
			tag := strings.Fields(strings.TrimPrefix(line, "<"))[0]
			stack = append(stack, frame{tag: tag})
			switch tag {
			case "ITEM":
				flush()
				inItem = true
			case "SOURCE":
				if inItem {
					inSource = true
				}
			}
			continue
		}
		if line == ">" {
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.tag == "ITEM" {
					flush()
				}
				if top.tag == "SOURCE" {
					inSource = false
				}
			}
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		key := parts[0]
		val := strings.TrimSpace(parts[1])
		atTrackRoot := len(stack) == 1 && stack[0].tag == "TRACK"

		switch {
		case key == "TRACKID" && atTrackRoot:
			t.GUID = strings.Trim(val, "{}")
		case key == "NAME" && atTrackRoot:
			t.Name = unquote(val)
		case key == "VOLPAN" && atTrackRoot:
			if v, err := strconv.ParseFloat(strings.Fields(val)[0], 64); err == nil {
				trackVol = v
			}
		case key == "VOLPAN" && inItem:
			if v, err := strconv.ParseFloat(strings.Fields(val)[0], 64); err == nil {
				itemVol = v
			}
		case key == "POSITION" && inItem:
			itemOffset, _ = strconv.ParseFloat(val, 64)
		case key == "FILE" && inSource:
			sourceFile = unquote(val)
			sourceFile = filepath.FromSlash(strings.ReplaceAll(sourceFile, "\\", "/"))
		}
	}
	flush()
	for i := range t.Items {
		t.Items[i].TrackName = t.Name
	}
	return t
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
