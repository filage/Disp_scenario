ALTER TABLE analysis_runs
  ADD COLUMN IF NOT EXISTS input_tokens bigint,
  ADD COLUMN IF NOT EXISTS output_tokens bigint,
  ADD COLUMN IF NOT EXISTS thinking_tokens bigint,
  ADD COLUMN IF NOT EXISTS total_tokens bigint,
  ADD COLUMN IF NOT EXISTS estimated_cost_usd numeric(14, 8),
  ADD COLUMN IF NOT EXISTS pricing_version text;
