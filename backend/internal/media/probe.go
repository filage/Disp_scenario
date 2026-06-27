package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/example/dispscenario-analyst-v2/internal/observability"
)

type Metadata struct {
	DurationSec float64
	Width       int
	Height      int
	Codec       string
}

type probeResponse struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
}

func Probe(ctx context.Context, path string) (metadata Metadata, err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("ffmpeg", "probe", started, err) }()
	output, err := exec.CommandContext(
		ctx,
		"ffprobe", "-v", "error", "-show_entries",
		"format=duration:stream=codec_type,codec_name,width,height",
		"-of", "json", path,
	).Output()
	if err != nil {
		return Metadata{}, fmt.Errorf("ffprobe: %w", err)
	}
	var response probeResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return Metadata{}, fmt.Errorf("decode ffprobe response: %w", err)
	}
	duration, _ := strconv.ParseFloat(response.Format.Duration, 64)
	metadata = Metadata{DurationSec: duration}
	for _, stream := range response.Streams {
		if stream.CodecType == "video" {
			metadata.Width = stream.Width
			metadata.Height = stream.Height
			metadata.Codec = stream.CodecName
			break
		}
	}
	return metadata, nil
}
