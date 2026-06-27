ALTER TABLE analysis_jobs
  ADD COLUMN correlation_id text;

CREATE INDEX analysis_jobs_correlation_id_idx
  ON analysis_jobs (correlation_id)
  WHERE correlation_id IS NOT NULL;
