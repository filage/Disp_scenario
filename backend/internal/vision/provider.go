package vision

import (
	"context"

	"github.com/google/uuid"

	"github.com/example/dispscenario-analyst-v2/internal/domain"
	"github.com/example/dispscenario-analyst-v2/internal/media"
)

type Result struct {
	Provider  string
	Model     string
	RawText   string
	RawEvents []domain.RawVisionEvent
	Usage     *Usage
	Cost      *CostEstimate
}

type Usage struct {
	InputTokens    int64
	OutputTokens   int64
	ThinkingTokens int64
	TotalTokens    int64
}

type CostEstimate struct {
	USD            float64
	PricingVersion string
}

type Provider interface {
	Extract(ctx context.Context, analysisRunID uuid.UUID, videoPath string, metadata media.Metadata) (Result, error)
}
