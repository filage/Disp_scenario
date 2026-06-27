package media

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/example/dispscenario-analyst-v2/internal/observability"
)

func ExtractEvidenceFrame(ctx context.Context, videoPath, outputPath string, timestampMS int) (err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("ffmpeg", "evidence_frame", started, err) }()
	if timestampMS < 0 {
		return fmt.Errorf("timestamp must be non-negative")
	}
	timestamp := strconv.FormatFloat(float64(timestampMS)/1000, 'f', 3, 64)
	output, err := exec.CommandContext(
		ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-ss", timestamp, "-i", videoPath, "-frames:v", "1",
		"-q:v", "2", outputPath,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg evidence frame: %w: %s", err, string(output))
	}
	return nil
}
