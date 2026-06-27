package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/jobs"
	"github.com/example/dispscenario-analyst-v2/internal/observability"
)

type Dispatcher struct {
	pool   *pgxpool.Pool
	queue  jobs.Queue
	logger *slog.Logger
}

func NewDispatcher(pool *pgxpool.Pool, queue jobs.Queue, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{pool: pool, queue: queue, logger: logger}
}

func (d *Dispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := d.dispatch(ctx); err != nil {
				d.logger.Error("outbox dispatch failed", "error", err)
			}
		}
	}
}

func (d *Dispatcher) dispatch(ctx context.Context) error {
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, payload
		FROM outbox_events
		WHERE published_at IS NULL AND available_at <= now()
		ORDER BY created_at
		LIMIT 20
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type event struct {
		id      uuid.UUID
		payload json.RawMessage
	}
	events := make([]event, 0)
	for rows.Next() {
		var item event
		if err := rows.Scan(&item.id, &item.payload); err != nil {
			return err
		}
		events = append(events, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range events {
		var envelope struct {
			CorrelationID string `json:"correlationId"`
		}
		_ = json.Unmarshal(item.payload, &envelope)
		taskID, enqueueErr := d.queue.EnqueueAnalysis(ctx, item.payload)
		if enqueueErr != nil {
			observability.OutboxEvents.WithLabelValues("failed").Inc()
			_, _ = tx.Exec(ctx, `
				UPDATE outbox_events
				SET attempts = attempts + 1, last_error = $2,
				    available_at = now() + interval '10 seconds'
				WHERE id = $1`, item.id, enqueueErr.Error())
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE outbox_events
			SET published_at = now(), attempts = attempts + 1, last_error = NULL
			WHERE id = $1`, item.id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE analysis_jobs SET queue_task_id = $2, updated_at = now()
			WHERE id = (SELECT aggregate_id FROM outbox_events WHERE id = $1)`,
			item.id, taskID); err != nil {
			return err
		}
		observability.OutboxEvents.WithLabelValues("published").Inc()
		d.logger.Info(
			"outbox event published",
			"outbox_id", item.id,
			"task_id", taskID,
			"correlation_id", envelope.CorrelationID,
		)
	}

	return tx.Commit(ctx)
}
