package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/example/dispscenario-analyst-v2/internal/observability"
)

const AnalysisTask = "analysis:process"

type AnalysisPayload struct {
	JobID         string `json:"jobId"`
	AnalysisRunID string `json:"analysisRunId"`
	RecordingID   string `json:"recordingId"`
	CorrelationID string `json:"correlationId"`
}

func DecodeAnalysisPayload(payload json.RawMessage) (AnalysisPayload, error) {
	var envelope AnalysisPayload
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return AnalysisPayload{}, fmt.Errorf("decode analysis payload: %w", err)
	}
	if envelope.JobID == "" || envelope.AnalysisRunID == "" || envelope.RecordingID == "" {
		return AnalysisPayload{}, fmt.Errorf("decode analysis payload: required ids are missing")
	}
	return envelope, nil
}

type Queue interface {
	EnqueueAnalysis(ctx context.Context, payload json.RawMessage) (string, error)
}

type AsynqQueue struct {
	client *asynq.Client
}

func NewAsynqQueue(client *asynq.Client) *AsynqQueue {
	return &AsynqQueue{client: client}
}

func (q *AsynqQueue) EnqueueAnalysis(ctx context.Context, payload json.RawMessage) (taskID string, err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("redis", "enqueue_analysis", started, err) }()
	task := asynq.NewTask(AnalysisTask, payload)
	envelope, err := DecodeAnalysisPayload(payload)
	if err != nil {
		return "", err
	}
	info, err := q.client.EnqueueContext(
		ctx,
		task,
		asynq.TaskID(envelope.JobID),
		asynq.Queue("analysis"),
		asynq.MaxRetry(5),
		asynq.Timeout(45*time.Minute),
		asynq.Retention(24*time.Hour),
	)
	if err != nil {
		return "", fmt.Errorf("enqueue analysis: %w", err)
	}
	return info.ID, nil
}
