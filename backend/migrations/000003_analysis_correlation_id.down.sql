DROP INDEX IF EXISTS analysis_jobs_correlation_id_idx;

ALTER TABLE analysis_jobs
  DROP COLUMN IF EXISTS correlation_id;
