package recording

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/example/dispscenario-analyst-v2/internal/database/db"
)

var ErrNotFound = errors.New("recording not found")

type Recording struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"-"`
	OriginalName   string    `json:"originalName"`
	MimeType       string    `json:"mimeType"`
	SizeBytes      int64     `json:"sizeBytes"`
	DurationSec    *float64  `json:"durationSec"`
	Status         string    `json:"status"`
	ObjectKey      string    `json:"-"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"-"`
}

type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: db.New(pool), pool: pool}
}

func (r *Repository) List(ctx context.Context, organizationID uuid.UUID) ([]Recording, error) {
	rows, err := r.queries.ListRecordings(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	items := make([]Recording, 0, len(rows))
	for _, row := range rows {
		items = append(items, fromDB(row))
	}
	return items, nil
}

func (r *Repository) Get(ctx context.Context, organizationID, id uuid.UUID) (Recording, error) {
	row, err := r.queries.GetRecording(ctx, db.GetRecordingParams{
		ID: id, OrganizationID: organizationID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Recording{}, ErrNotFound
	}
	return fromDB(row), err
}

func (r *Repository) Create(
	ctx context.Context,
	organizationID uuid.UUID,
	originalName, mimeType string,
	sizeBytes int64,
	objectKey, actor string,
) (Recording, error) {
	row, err := r.queries.CreateRecording(ctx, db.CreateRecordingParams{
		OrganizationID: organizationID,
		OriginalName:   originalName, MimeType: mimeType, SizeBytes: sizeBytes,
		ObjectKey: objectKey, CreatedBy: pgtype.Text{String: actor, Valid: actor != ""},
	})
	return fromDB(row), err
}

func (r *Repository) Complete(
	ctx context.Context,
	organizationID, id uuid.UUID,
	actor string,
) (Recording, error) {
	row, err := r.queries.CompleteRecordingUpload(ctx, db.CompleteRecordingUploadParams{
		ID: id, OrganizationID: organizationID,
		UpdatedBy: pgtype.Text{String: actor, Valid: actor != ""},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Recording{}, ErrNotFound
	}
	return fromDB(row), err
}

func (r *Repository) Delete(ctx context.Context, organizationID, id uuid.UUID) (Recording, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Recording{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := r.queries.WithTx(tx).DeleteRecording(ctx, db.DeleteRecordingParams{
		ID: id, OrganizationID: organizationID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Recording{}, ErrNotFound
	}
	if err != nil {
		return Recording{}, err
	}

	// Scenario templates aggregate data across recordings and therefore do not
	// reference a recording directly. Cascading the recording deletion removes
	// its instances; remove templates that no longer have any source instances.
	// automation_candidates are deleted by their template FK cascade.
	if _, err := tx.Exec(ctx, `
		DELETE FROM scenario_templates t
		WHERE t.organization_id = $1
		  AND NOT EXISTS (
		    SELECT 1 FROM scenario_instances i WHERE i.template_id = t.id
		  )`, organizationID); err != nil {
		return Recording{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Recording{}, err
	}
	return fromDB(row), nil
}

func fromDB(row db.Recording) Recording {
	var duration *float64
	if row.DurationSec.Valid {
		duration = &row.DurationSec.Float64
	}
	return Recording{
		ID: row.ID, OrganizationID: row.OrganizationID,
		OriginalName: row.OriginalName, MimeType: row.MimeType,
		SizeBytes: row.SizeBytes, DurationSec: duration,
		Status: string(row.Status), ObjectKey: row.ObjectKey,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}
