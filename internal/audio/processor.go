package audio

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

type Processor struct {
	ffmpegPath  string
	ffprobePath string
	numPeaks    int
}

type Metadata struct {
	DurationMS int64  `json:"duration_ms"`
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
}

func NewProcessor(ffmpegPath, ffprobePath string, numPeaks int) *Processor {
	return &Processor{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		numPeaks:    numPeaks,
	}
}

func (p *Processor) Probe(ctx context.Context, filePath string) (*Metadata, error) {
	cmd := exec.CommandContext(ctx, p.ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}

	var probe struct {
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			Duration   string `json:"duration"`
			SampleRate string `json:"sample_rate"`
			CodecName  string `json:"codec_name"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	dur, _ := strconv.ParseFloat(probe.Format.Duration, 64)
	meta := &Metadata{
		DurationMS: int64(dur * 1000),
		Format:     probe.Format.Name,
	}

	for _, s := range probe.Streams {
		if s.CodecType == "audio" {
			meta.SampleRate, _ = strconv.Atoi(s.SampleRate)
			meta.Format = s.CodecName
			// WebM/MediaRecorder files often omit format-level duration; fall back to stream duration
			if meta.DurationMS <= 0 {
				if d, err := strconv.ParseFloat(s.Duration, 64); err == nil && d > 0 {
					meta.DurationMS = int64(d * 1000)
				}
			}
			break
		}
	}

	return meta, nil
}

func (p *Processor) GeneratePeaks(ctx context.Context, filePath string) (json.RawMessage, error) {
	// Decode audio to raw mono float32 samples at a reduced sample rate
	// Using 8000 Hz is sufficient for waveform visualization
	cmd := exec.CommandContext(ctx, p.ffmpegPath,
		"-i", filePath,
		"-ac", "1",
		"-ar", "8000",
		"-f", "f32le",
		"-acodec", "pcm_f32le",
		"pipe:1",
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg decode: %w", err)
	}

	numSamples := len(out) / 4
	if numSamples == 0 {
		return json.Marshal([]float64{})
	}

	samplesPerPeak := numSamples / p.numPeaks
	if samplesPerPeak < 1 {
		samplesPerPeak = 1
	}

	peaks := make([]float64, 0, p.numPeaks)
	for i := 0; i < p.numPeaks && i*samplesPerPeak < numSamples; i++ {
		start := i * samplesPerPeak
		end := start + samplesPerPeak
		if end > numSamples {
			end = numSamples
		}

		var maxVal float64
		for j := start; j < end; j++ {
			bits := binary.LittleEndian.Uint32(out[j*4 : j*4+4])
			sample := math.Float32frombits(bits)
			abs := math.Abs(float64(sample))
			if abs > maxVal {
				maxVal = abs
			}
		}
		peaks = append(peaks, maxVal)
	}

	// Normalize peaks to 0..1 range
	var globalMax float64
	for _, v := range peaks {
		if v > globalMax {
			globalMax = v
		}
	}
	if globalMax > 0 {
		for i := range peaks {
			peaks[i] /= globalMax
		}
	}

	return json.Marshal(peaks)
}

// NeedsTranscode returns true for lossless formats that should be transcoded to Opus for streaming.
func (p *Processor) NeedsTranscode(format string) bool {
	if strings.HasPrefix(format, "pcm_") {
		return true
	}
	return format == "flac" || format == "alac"
}

// TranscodeToOpus encodes srcPath to Opus (Ogg container) at 128kbps VBR and writes to dstPath.
func (p *Processor) TranscodeToOpus(ctx context.Context, srcPath, dstPath string) error {
	cmd := exec.CommandContext(ctx, p.ffmpegPath,
		"-i", srcPath,
		"-c:a", "libopus",
		"-b:a", "128k",
		"-vbr", "on",
		"-y",
		dstPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg transcode: %w: %s", err, out)
	}
	return nil
}

func (p *Processor) IsSupported(filename string) bool {
	ext := strings.ToLower(filename)
	for _, e := range []string{".mp3", ".wav", ".flac", ".ogg", ".aac", ".m4a", ".aiff", ".aif", ".wma", ".opus", ".webm"} {
		if strings.HasSuffix(ext, e) {
			return true
		}
	}
	return false
}
