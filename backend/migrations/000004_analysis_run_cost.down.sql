ALTER TABLE analysis_runs
  DROP COLUMN IF EXISTS pricing_version,
  DROP COLUMN IF EXISTS estimated_cost_usd,
  DROP COLUMN IF EXISTS total_tokens,
  DROP COLUMN IF EXISTS thinking_tokens,
  DROP COLUMN IF EXISTS output_tokens,
  DROP COLUMN IF EXISTS input_tokens;
