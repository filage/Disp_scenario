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
	var envelope struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", fmt.Errorf("decode analysis payload: %w", err)
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
