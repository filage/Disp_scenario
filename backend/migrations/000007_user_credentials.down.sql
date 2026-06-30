DROP TABLE IF EXISTS provider_credentials;

ALTER TABLE analysis_jobs
  DROP COLUMN IF EXISTS requested_by;
