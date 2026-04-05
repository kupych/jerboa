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
	inItem := false
	inSource := false
	itemOffset := 0.0
	sourceFile := ""

	flush := func() {
		if inItem && sourceFile != "" {
			abs := sourceFile
			if !filepath.IsAbs(sourceFile) {
				abs = filepath.Join(baseDir, sourceFile)
			}
			items = append(items, RPPItem{
				TrackName: currentTrackName,
				FilePath:  abs,
				OffsetMS:  int64(math.Round(itemOffset * 1000)),
			})
		}
		inItem = false
		inSource = false
		sourceFile = ""
		itemOffset = 0
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

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

		switch {
		case key == "NAME" && len(stack) > 0 && stack[len(stack)-1].tag == "TRACK":
			currentTrackName = unquote(val)
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

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
