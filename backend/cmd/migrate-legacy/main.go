package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/database"
	"github.com/example/dispscenario-analyst-v2/internal/platform"
	"github.com/example/dispscenario-analyst-v2/internal/storage"
)

var migrationNamespace = uuid.MustParse("6d58e41f-f187-4a19-98f1-ff086bc9d688")

type field struct {
	old       string
	new       string
	transform func(any) any
}

type tableSpec struct {
	oldTable string
	newTable string
	fields   []field
}

type legacyStore map[string][]map[string]any

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()
	legacyURL := os.Getenv("LEGACY_DATABASE_URL")
	targetURL := os.Getenv("DATABASE_URL")
	if targetURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	apply, _ := strconv.ParseBool(os.Getenv("MIGRATION_APPLY"))
	target, err := database.OpenPool(ctx, targetURL)
	if err != nil {
		logger.Error("connect target database", "error", err)
		os.Exit(1)
	}
	defer target.Close()

	var objectStorage *storage.Storage
	if apply {
		objectStorage, err = storage.New(
			value("S3_ENDPOINT", "http://localhost:9000"),
			value("S3_PUBLIC_ENDPOINT", "http://localhost:9000"),
			os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"),
			value("S3_BUCKET", "analyst-recordings"),
			value("S3_REGION", "us-east-1"),
			value("S3_USE_SSL", "false") == "true",
		)
		if err != nil {
			logger.Error("configure target storage", "error", err)
			os.Exit(1)
		}
		if err := objectStorage.EnsureBucket(ctx); err != nil {
			logger.Error("initialize target storage", "error", err)
			os.Exit(1)
		}
	}

	counts := map[string]int{}
	if legacyURL != "" {
		legacy, connectErr := pgxpool.New(ctx, legacyURL)
		if connectErr != nil {
			logger.Error("connect legacy database", "error", connectErr)
			os.Exit(1)
		}
		defer legacy.Close()
		counts, err = migrateDatabase(ctx, legacy, target, objectStorage, apply)
	} else {
		storePath := value("LEGACY_STORE_PATH", filepath.Join(value("LEGACY_ROOT", "../analyst-app"), "server", "data", "store.json"))
		var store legacyStore
		store, err = loadLegacyStore(storePath)
		if err == nil {
			counts, err = migrateStore(ctx, store, target, objectStorage, apply)
		}
	}
	if err != nil {
		logger.Error("legacy migration failed", "error", err)
		os.Exit(1)
	}
	if apply {
		if err := verifyTarget(ctx, target, objectStorage, counts); err != nil {
			logger.Error("legacy migration verification failed", "error", err)
			os.Exit(1)
		}
	}
	logger.Info("legacy migration completed", "apply", apply, "counts", counts)
}

func migrateDatabase(
	ctx context.Context,
	legacy, target *pgxpool.Pool,
	objectStorage *storage.Storage,
	apply bool,
) (map[string]int, error) {
	counts := map[string]int{}
	var err error
	counts["recordings"], err = migrateRecordings(ctx, legacy, target, objectStorage, apply)
	if err != nil {
		return counts, fmt.Errorf("recordings: %w", err)
	}
	counts["analysis_jobs"], err = migrateJobsFromDatabase(ctx, legacy, target, apply)
	if err != nil {
		return counts, fmt.Errorf("analysis jobs: %w", err)
	}
	for _, spec := range specs() {
		count, migrationErr := migrateTable(ctx, legacy, target, spec, apply)
		if migrationErr != nil {
			return counts, fmt.Errorf("%s: %w", spec.oldTable, migrationErr)
		}
		counts[spec.newTable] = count
	}
	return counts, nil
}

func loadLegacyStore(path string) (legacyStore, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open legacy store %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	var store legacyStore
	if err := decoder.Decode(&store); err != nil {
		return nil, fmt.Errorf("decode legacy store %s: %w", path, err)
	}
	return store, nil
}

func migrateStore(
	ctx context.Context,
	store legacyStore,
	target *pgxpool.Pool,
	objectStorage *storage.Storage,
	apply bool,
) (map[string]int, error) {
	counts := map[string]int{}
	recordings := store["videos"]
	counts["recordings"] = len(recordings)
	for index, item := range recordings {
		if err := migrateStoreRecording(ctx, target, objectStorage, item, apply); err != nil {
			return counts, fmt.Errorf("videos row %d: %w", index+1, err)
		}
	}
	jobs := store["jobs"]
	counts["analysis_jobs"] = len(jobs)
	for index, item := range jobs {
		if err := migrateStoreJob(ctx, target, item, apply); err != nil {
			return counts, fmt.Errorf("jobs row %d: %w", index+1, err)
		}
	}
	for _, spec := range specs() {
		rows := store[storeKey(spec.oldTable)]
		counts[spec.newTable] = len(rows)
		for index, item := range rows {
			if err := migrateStoreRow(ctx, target, spec, item, apply); err != nil {
				return counts, fmt.Errorf("%s row %d: %w", storeKey(spec.oldTable), index+1, err)
			}
		}
	}
	groundTruth := store["groundTruth"]
	counts["ground_truth_events"] = len(groundTruth)
	for index, item := range groundTruth {
		if err := migrateGroundTruth(ctx, target, item, apply); err != nil {
			return counts, fmt.Errorf("groundTruth row %d: %w", index+1, err)
		}
	}
	return counts, nil
}

func migrateRecordings(
	ctx context.Context,
	legacy, target *pgxpool.Pool,
	objectStorage *storage.Storage,
	apply bool,
) (int, error) {
	rows, err := legacy.Query(ctx, `
		SELECT "id","fileName","originalName","mimeType","sizeBytes","durationSec",
		       "source","status"::text,"filePath","createdAt","updatedAt"
		FROM "VideoRecording" ORDER BY "createdAt"`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var legacyID, fileName, originalName, mimeType, source, status, filePath string
		var sizeBytes int64
		var duration *float64
		var createdAt, updatedAt any
		if err := rows.Scan(
			&legacyID, &fileName, &originalName, &mimeType, &sizeBytes, &duration,
			&source, &status, &filePath, &createdAt, &updatedAt,
		); err != nil {
			return count, err
		}
		count++
		if !apply {
			continue
		}
		id := mapped("recording", legacyID)
		extension := ".mp4"
		if mimeType == "video/webm" {
			extension = ".webm"
		}
		objectKey := fmt.Sprintf("recordings/%s/source%s", id, extension)
		resolvedPath := resolveLegacyPath(filePath)
		if _, err := os.Stat(resolvedPath); err != nil {
			return count, fmt.Errorf("legacy video %s (%s): %w", legacyID, resolvedPath, err)
		}
		if err := objectStorage.Upload(ctx, objectKey, resolvedPath, mimeType); err != nil {
			return count, fmt.Errorf("upload legacy video %s: %w", legacyID, err)
		}
		if _, err := target.Exec(ctx, `
			INSERT INTO recordings (
				id,organization_id,original_name,mime_type,size_bytes,duration_sec,
				status,source,object_key,created_by,updated_by,created_at,updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'legacy-migration','legacy-migration',$10,$11)
			ON CONFLICT (id) DO NOTHING`,
			id, platform.LocalOrganizationID, originalName, mimeType, sizeBytes, duration,
			status, source, objectKey, createdAt, updatedAt); err != nil {
			return count, err
		}
	}
	return count, rows.Err()
}

func migrateJobsFromDatabase(ctx context.Context, legacy, target *pgxpool.Pool, apply bool) (int, error) {
	rows, err := legacy.Query(ctx, `
		SELECT "id","videoId","status"::text,"provider","model","error",
		       "startedAt","completedAt","createdAt"
		FROM "AnalysisJob" ORDER BY "createdAt"`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var legacyID, videoID, status, provider string
		var model, jobError *string
		var startedAt, completedAt any
		var createdAt time.Time
		if err := rows.Scan(
			&legacyID, &videoID, &status, &provider, &model, &jobError,
			&startedAt, &completedAt, &createdAt,
		); err != nil {
			return count, err
		}
		count++
		if !apply {
			continue
		}
		item := map[string]any{
			"id": legacyID, "videoId": videoID, "status": status, "provider": provider,
			"model": model, "error": jobError, "startedAt": startedAt,
			"completedAt": completedAt, "createdAt": createdAt,
		}
		if err := migrateStoreJob(ctx, target, item, true); err != nil {
			return count, err
		}
	}
	return count, rows.Err()
}

func migrateTable(
	ctx context.Context,
	legacy, target *pgxpool.Pool,
	spec tableSpec,
	apply bool,
) (int, error) {
	oldColumns := make([]string, len(spec.fields))
	newColumns := make([]string, len(spec.fields))
	for index, current := range spec.fields {
		oldColumns[index] = quote(current.old)
		newColumns[index] = current.new
	}
	rows, err := legacy.Query(ctx, fmt.Sprintf(
		`SELECT %s FROM %s`, strings.Join(oldColumns, ","), quote(spec.oldTable),
	))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return count, err
		}
		for index, current := range spec.fields {
			if current.transform != nil {
				values[index] = current.transform(values[index])
			}
		}
		count++
		if !apply {
			continue
		}
		placeholders := make([]string, len(values))
		for index := range values {
			placeholders[index] = fmt.Sprintf("$%d", index+1)
		}
		query := fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
			spec.newTable, strings.Join(newColumns, ","), strings.Join(placeholders, ","),
		)
		if _, err := target.Exec(ctx, query, values...); err != nil {
			return count, fmt.Errorf("%s row %d: %w", spec.oldTable, count, err)
		}
	}
	return count, rows.Err()
}

func migrateStoreRecording(
	ctx context.Context,
	target *pgxpool.Pool,
	objectStorage *storage.Storage,
	item map[string]any,
	apply bool,
) error {
	if !apply {
		return nil
	}
	legacyID := stringValue(item["id"])
	mimeType := stringValue(item["mimeType"])
	extension := ".mp4"
	if mimeType == "video/webm" {
		extension = ".webm"
	}
	id := mapped("recording", legacyID)
	objectKey := fmt.Sprintf("recordings/%s/source%s", id, extension)
	resolvedPath := resolveLegacyPath(stringValue(item["filePath"]))
	if _, err := os.Stat(resolvedPath); err != nil {
		return fmt.Errorf("legacy video %s (%s): %w", legacyID, resolvedPath, err)
	}
	if err := objectStorage.Upload(ctx, objectKey, resolvedPath, mimeType); err != nil {
		return fmt.Errorf("upload legacy video %s: %w", legacyID, err)
	}
	_, err := target.Exec(ctx, `
		INSERT INTO recordings (
			id,organization_id,original_name,mime_type,size_bytes,duration_sec,
			status,source,object_key,created_by,updated_by,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'legacy-migration','legacy-migration',$10,$11)
		ON CONFLICT (id) DO UPDATE SET
			original_name=EXCLUDED.original_name, mime_type=EXCLUDED.mime_type,
			size_bytes=EXCLUDED.size_bytes, duration_sec=EXCLUDED.duration_sec,
			status=EXCLUDED.status, source=EXCLUDED.source, object_key=EXCLUDED.object_key,
			updated_at=EXCLUDED.updated_at`,
		id, platform.LocalOrganizationID, stringValue(item["originalName"]), mimeType,
		numberValue(item["sizeBytes"]), nullableNumber(item["durationSec"]),
		stringValue(item["status"]), defaultString(item["source"], "upload"), objectKey,
		timeValue(item["createdAt"]), timeValue(item["updatedAt"]),
	)
	return err
}

func migrateStoreJob(ctx context.Context, target *pgxpool.Pool, item map[string]any, apply bool) error {
	if !apply {
		return nil
	}
	legacyID := stringValue(item["id"])
	recordingID := mapped("recording", stringValue(item["videoId"]))
	runID := mapped("job-run", legacyID)
	jobID := mapped("job", legacyID)
	status := defaultString(item["status"], "QUEUED")
	provider := defaultString(item["provider"], "gemini")
	createdAt := timeValue(item["createdAt"])
	startedAt := nullableTime(item["startedAt"])
	completedAt := nullableTime(item["completedAt"])
	errorValue := nullableString(item["error"])
	if _, err := target.Exec(ctx, `
		INSERT INTO analysis_runs (
			id,organization_id,recording_id,provider,model,prompt_version,
			normalization_version,grouping_version,status,error,started_at,
			completed_at,created_by,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,'legacy-job-migration-v1',
		          'legacy-job-migration-v1','legacy-job-migration-v1',$6,$7,$8,$9,
		          'legacy-migration',$10,$10)
		ON CONFLICT (id) DO NOTHING`,
		runID, platform.LocalOrganizationID, recordingID, provider, nullableString(item["model"]),
		status, errorValue, startedAt, completedAt, createdAt,
	); err != nil {
		return err
	}
	progress := 0
	switch status {
	case "PROCESSING":
		progress = 50
	case "COMPLETED", "FAILED":
		progress = 100
	}
	_, err := target.Exec(ctx, `
		INSERT INTO analysis_jobs (
			id,organization_id,recording_id,analysis_run_id,status,progress,
			idempotency_key,last_error,started_at,completed_at,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)
		ON CONFLICT (id) DO NOTHING`,
		jobID, platform.LocalOrganizationID, recordingID, runID, status, progress,
		"legacy-job:"+legacyID, errorValue, startedAt, completedAt, createdAt,
	)
	return err
}

func migrateStoreRow(
	ctx context.Context,
	target *pgxpool.Pool,
	spec tableSpec,
	item map[string]any,
	apply bool,
) error {
	if !apply {
		return nil
	}
	values := make([]any, len(spec.fields))
	columns := make([]string, len(spec.fields))
	placeholders := make([]string, len(spec.fields))
	for index, current := range spec.fields {
		value := normalizeStoreValue(current.old, item[current.old])
		if current.transform != nil {
			value = current.transform(value)
		}
		values[index] = value
		columns[index] = current.new
		placeholders[index] = fmt.Sprintf("$%d", index+1)
	}
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		spec.newTable, strings.Join(columns, ","), strings.Join(placeholders, ","),
	)
	_, err := target.Exec(ctx, query, values...)
	return err
}

func migrateGroundTruth(ctx context.Context, target *pgxpool.Pool, item map[string]any, apply bool) error {
	if !apply {
		return nil
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	legacyID := defaultString(item["id"], string(payload))
	var recordingID any
	if videoID := stringValue(item["videoId"]); videoID != "" {
		candidate := mapped("recording", videoID)
		var exists bool
		if err := target.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM recordings WHERE id=$1)`, candidate).Scan(&exists); err != nil {
			return err
		}
		if exists {
			recordingID = candidate
		}
	}
	_, err = target.Exec(ctx, `
		INSERT INTO ground_truth_events (
			id,organization_id,recording_id,timestamp_ms,payload,created_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO NOTHING`,
		mapped("ground-truth", legacyID), platform.LocalOrganizationID, recordingID,
		numberValue(item["timestampMs"]), payload, timeValue(item["savedAt"]),
	)
	return err
}

func verifyTarget(
	ctx context.Context,
	target *pgxpool.Pool,
	objectStorage *storage.Storage,
	counts map[string]int,
) error {
	for table, expected := range counts {
		if expected == 0 {
			continue
		}
		var actual int
		query := fmt.Sprintf("SELECT count(*) FROM %s", quote(table))
		if err := target.QueryRow(ctx, query).Scan(&actual); err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}
		if actual < expected {
			return fmt.Errorf("count %s: expected at least %d, got %d", table, expected, actual)
		}
	}
	orphanChecks := map[string]string{
		"analysis_runs.recording_id":  `SELECT count(*) FROM analysis_runs r LEFT JOIN recordings v ON v.id=r.recording_id WHERE v.id IS NULL`,
		"analysis_jobs.run_id":        `SELECT count(*) FROM analysis_jobs j LEFT JOIN analysis_runs r ON r.id=j.analysis_run_id WHERE r.id IS NULL`,
		"action_events.recording_id":  `SELECT count(*) FROM action_events e LEFT JOIN recordings v ON v.id=e.recording_id WHERE v.id IS NULL`,
		"scenario_instances.template": `SELECT count(*) FROM scenario_instances i LEFT JOIN scenario_templates t ON t.id=i.template_id WHERE i.template_id IS NOT NULL AND t.id IS NULL`,
	}
	for name, query := range orphanChecks {
		var count int
		if err := target.QueryRow(ctx, query).Scan(&count); err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
		if count != 0 {
			return fmt.Errorf("verify %s: found %d orphan rows", name, count)
		}
	}
	rows, err := target.Query(ctx, `SELECT object_key FROM recordings`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return err
		}
		if _, err := objectStorage.Stat(ctx, key); err != nil {
			return fmt.Errorf("verify S3 object %s: %w", key, err)
		}
	}
	return rows.Err()
}

func specs() []tableSpec {
	id := func(kind string) func(any) any {
		return func(value any) any {
			if value == nil {
				return nil
			}
			return mapped(kind, fmt.Sprint(value))
		}
	}
	idArray := func(kind string) func(any) any {
		return func(value any) any {
			if value == nil {
				return []byte("[]")
			}
			data, _ := json.Marshal(value)
			var source []string
			if err := json.Unmarshal(data, &source); err != nil {
				return value
			}
			result := make([]uuid.UUID, len(source))
			for index, current := range source {
				result[index] = mapped(kind, current)
			}
			encoded, _ := json.Marshal(result)
			return encoded
		}
	}
	jsonValue := func(fallback string) func(any) any {
		return func(value any) any {
			if value == nil {
				return []byte(fallback)
			}
			return value
		}
	}
	org := func(any) any { return platform.LocalOrganizationID }
	return []tableSpec{
		{"AnalysisRun", "analysis_runs", []field{
			{"id", "id", id("run")}, {"videoId", "recording_id", id("recording")},
			{"videoId", "organization_id", org}, {"provider", "provider", nil},
			{"model", "model", nil}, {"promptVersion", "prompt_version", nil},
			{"normalizationVersion", "normalization_version", nil},
			{"groupingVersion", "grouping_version", nil}, {"status", "status", nil},
			{"rawText", "raw_text", nil}, {"error", "error", nil},
			{"startedAt", "started_at", nil}, {"completedAt", "completed_at", nil},
			{"createdAt", "created_at", nil}, {"createdAt", "updated_at", nil},
		}},
		{"RawVisionEvent", "raw_vision_events", []field{
			{"id", "id", id("raw")}, {"analysisRunId", "analysis_run_id", id("run")},
			{"timestampMs", "timestamp_ms", nil}, {"screen", "screen", nil},
			{"visibleText", "visible_text", nil}, {"target", "target", nil},
			{"eventTypeGuess", "event_type_guess", nil}, {"colorCues", "color_cues", jsonValue("[]")},
			{"stateChange", "state_change", nil}, {"confidence", "confidence", nil},
			{"payload", "payload", jsonValue("{}")}, {"createdAt", "created_at", nil},
		}},
		{"ActionEvent", "action_events", []field{
			{"id", "id", id("action")}, {"videoId", "recording_id", id("recording")},
			{"analysisRunId", "analysis_run_id", id("run")}, {"timestampMs", "timestamp_ms", nil},
			{"canonicalAction", "canonical_action", nil}, {"eventType", "event_type", nil},
			{"screen", "screen", nil}, {"entityType", "entity_type", nil},
			{"entityId", "entity_id", nil}, {"orderId", "order_id", nil},
			{"issueType", "issue_type", nil}, {"target", "target", nil},
			{"confidence", "confidence", nil}, {"sourceRawEventIds", "source_raw_event_ids", idArray("raw")},
			{"qualityFlags", "quality_flags", jsonValue("[]")}, {"payload", "payload", jsonValue("{}")},
			{"source", "source", nil}, {"qaStatus", "qa_status", nil},
			{"qaComment", "qa_comment", nil}, {"createdAt", "created_at", nil},
			{"updatedAt", "updated_at", nil},
		}},
		{"ScenarioTemplate", "scenario_templates", []field{
			{"id", "id", id("template")}, {"id", "organization_id", org},
			{"code", "code", nil}, {"name", "name", nil}, {"issueType", "issue_type", nil},
			{"signature", "signature", nil}, {"frequency", "frequency", nil},
			{"averageDurationMs", "average_duration_ms", nil},
			{"medianDurationMs", "median_duration_ms", nil}, {"p95DurationMs", "p95_duration_ms", nil},
			{"manualCheckCount", "manual_check_count", nil},
			{"repeatedActionCount", "repeated_action_count", nil},
			{"confidenceAverage", "confidence_average", nil},
			{"ambiguousCount", "ambiguous_count", nil}, {"automationScore", "automation_score", nil},
			{"actionSequence", "action_sequence", jsonValue("[]")}, {"metrics", "metrics", jsonValue("{}")},
			{"status", "status", nil}, {"createdAt", "created_at", nil}, {"updatedAt", "updated_at", nil},
		}},
		{"ScenarioInstance", "scenario_instances", []field{
			{"id", "id", id("instance")}, {"videoId", "recording_id", id("recording")},
			{"analysisRunId", "analysis_run_id", id("run")}, {"templateId", "template_id", id("template")},
			{"knownScenarioCode", "known_scenario_code", nil}, {"orderId", "order_id", nil},
			{"entityType", "entity_type", nil}, {"entityId", "entity_id", nil},
			{"issueType", "issue_type", nil}, {"startedAtMs", "started_at_ms", nil},
			{"endedAtMs", "ended_at_ms", nil}, {"durationMs", "duration_ms", nil},
			{"eventIds", "event_ids", idArray("action")}, {"outcome", "outcome", nil},
			{"status", "status", nil}, {"confidence", "confidence", nil},
			{"boundaryRuleVersion", "boundary_rule_version", nil}, {"qualityFlags", "quality_flags", jsonValue("[]")},
			{"createdAt", "created_at", nil},
		}},
		{"AutomationCandidate", "automation_candidates", []field{
			{"id", "id", id("candidate")}, {"templateId", "template_id", id("template")},
			{"title", "title", nil}, {"type", "type", nil}, {"rationale", "rationale", nil},
			{"affectedSteps", "affected_steps", jsonValue("[]")}, {"impact", "impact", nil},
			{"confidence", "confidence", nil}, {"score", "score", nil},
			{"status", "status", nil}, {"breakdown", "breakdown", jsonValue("{}")}, {"createdAt", "created_at", nil},
		}},
		{"DataQualityIssue", "data_quality_issues", []field{
			{"id", "id", id("issue")}, {"videoId", "recording_id", id("recording")},
			{"analysisRunId", "analysis_run_id", id("run")}, {"rawVisionEventId", "raw_vision_event_id", id("raw")},
			{"actionEventId", "action_event_id", id("action")}, {"type", "type", nil},
			{"severity", "severity", nil}, {"message", "message", nil},
			{"timestampMs", "timestamp_ms", nil}, {"resolved", "resolved", nil},
			{"payload", "payload", jsonValue("{}")}, {"createdAt", "created_at", nil}, {"createdAt", "updated_at", nil},
		}},
		{"AnalystReport", "analyst_reports", []field{
			{"id", "id", id("report")}, {"videoId", "recording_id", id("recording")},
			{"analysisRunId", "analysis_run_id", id("run")}, {"summary", "summary", nil},
			{"observations", "observations", jsonValue("[]")}, {"recommendations", "recommendations", jsonValue("[]")},
			{"metrics", "metrics", jsonValue("{}")}, {"graphSummary", "graph_summary", jsonValue("{}")},
			{"model", "model", nil}, {"provider", "provider", nil},
			{"promptVersion", "prompt_version", nil},
			{"normalizationVersion", "normalization_version", nil},
			{"groupingVersion", "grouping_version", nil}, {"createdAt", "created_at", nil},
		}},
		{"ScenarioGraph", "scenario_graphs", []field{
			{"id", "id", id("graph")}, {"videoId", "recording_id", id("recording")},
			{"analysisRunId", "analysis_run_id", id("run")}, {"graph", "graph", jsonValue(`{"nodes":[],"edges":[]}`)},
			{"metrics", "metrics", jsonValue("{}")}, {"createdAt", "created_at", nil},
		}},
		{"KnownScenario", "known_scenarios", []field{
			{"code", "organization_id", org}, {"code", "code", nil}, {"name", "name", nil},
			{"issueType", "issue_type", nil}, {"entityType", "entity_type", nil},
			{"startActions", "start_actions", jsonValue("[]")}, {"requiredActions", "required_actions", jsonValue("[]")},
			{"optionalActions", "optional_actions", jsonValue("[]")}, {"endActions", "end_actions", jsonValue("[]")},
			{"forbiddenActions", "forbidden_actions", jsonValue("[]")}, {"timeoutMs", "timeout_ms", nil},
			{"version", "version", nil}, {"enabled", "enabled", nil},
			{"createdAt", "created_at", nil}, {"updatedAt", "updated_at", nil},
		}},
		{"BoundaryRule", "boundary_rules", []field{
			{"id", "organization_id", org}, {"id", "id", nil}, {"name", "name", nil},
			{"priority", "priority", nil}, {"type", "type", nil},
			{"conditions", "conditions", jsonValue("{}")}, {"version", "version", nil},
			{"enabled", "enabled", nil}, {"createdAt", "created_at", nil}, {"updatedAt", "updated_at", nil},
		}},
	}
}

func mapped(kind, legacyID string) uuid.UUID {
	return uuid.NewSHA1(migrationNamespace, []byte(kind+":"+legacyID))
}

func quote(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func resolveLegacyPath(path string) string {
	if path == "" {
		return path
	}
	root := value("LEGACY_ROOT", "../analyst-app")
	if isWindowsAbsolutePath(path) {
		normalized := strings.ReplaceAll(path, "\\", "/")
		lower := strings.ToLower(normalized)
		if index := strings.LastIndex(lower, "/analyst-app/"); index >= 0 {
			suffix := normalized[index+len("/analyst-app/"):]
			return filepath.Join(root, filepath.FromSlash(suffix))
		}
		if index := strings.Index(lower, "/uploads/"); index >= 0 {
			suffix := strings.TrimPrefix(normalized[index:], "/")
			return filepath.Join(root, filepath.FromSlash(suffix))
		}
		return filepath.Join(root, filepath.Base(normalized))
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func isWindowsAbsolutePath(path string) bool {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return true
	}
	return len(path) >= 3 &&
		path[1] == ':' &&
		((path[2] == '\\') || (path[2] == '/')) &&
		((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z'))
}

func storeKey(model string) string {
	return map[string]string{
		"AnalysisRun": "analysisRuns", "RawVisionEvent": "rawVisionEvents",
		"ActionEvent": "events", "ScenarioTemplate": "scenarioTemplates",
		"ScenarioInstance": "scenarioInstances", "AutomationCandidate": "automationCandidates",
		"DataQualityIssue": "dataQualityIssues", "AnalystReport": "reports",
		"ScenarioGraph": "scenarioGraphs", "KnownScenario": "knownScenarios",
		"BoundaryRule": "boundaryRules",
	}[model]
}

func normalizeStoreValue(fieldName string, current any) any {
	if current == nil {
		return nil
	}
	if strings.HasSuffix(fieldName, "At") || fieldName == "savedAt" {
		return timeValue(current)
	}
	switch value := current.(type) {
	case json.Number:
		if integer, err := value.Int64(); err == nil {
			return integer
		}
		floating, _ := value.Float64()
		return floating
	case map[string]any, []any:
		encoded, _ := json.Marshal(value)
		return encoded
	default:
		return current
	}
}

func stringValue(current any) string {
	if current == nil {
		return ""
	}
	if pointer, ok := current.(*string); ok {
		if pointer == nil {
			return ""
		}
		return *pointer
	}
	return fmt.Sprint(current)
}

func defaultString(current any, fallback string) string {
	if value := strings.TrimSpace(stringValue(current)); value != "" {
		return value
	}
	return fallback
}

func nullableString(current any) any {
	if value := strings.TrimSpace(stringValue(current)); value != "" {
		return value
	}
	return nil
}

func numberValue(current any) int64 {
	switch value := current.(type) {
	case json.Number:
		result, _ := value.Int64()
		return result
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		result, _ := strconv.ParseInt(fmt.Sprint(current), 10, 64)
		return result
	}
}

func nullableNumber(current any) any {
	if current == nil {
		return nil
	}
	switch value := current.(type) {
	case json.Number:
		result, _ := value.Float64()
		return result
	case float64:
		return value
	default:
		result, err := strconv.ParseFloat(fmt.Sprint(current), 64)
		if err != nil {
			return nil
		}
		return result
	}
}

func timeValue(current any) time.Time {
	if current == nil {
		return time.Now().UTC()
	}
	switch value := current.(type) {
	case time.Time:
		return value
	case *time.Time:
		if value != nil {
			return *value
		}
	default:
		if parsed, err := time.Parse(time.RFC3339Nano, fmt.Sprint(current)); err == nil {
			return parsed
		}
	}
	return time.Now().UTC()
}

func nullableTime(current any) any {
	if current == nil {
		return nil
	}
	return timeValue(current)
}

func value(name, fallback string) string {
	if current := os.Getenv(name); current != "" {
		return current
	}
	return fallback
}
