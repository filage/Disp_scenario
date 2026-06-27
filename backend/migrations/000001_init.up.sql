CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE recording_status AS ENUM (
  'PENDING_UPLOAD',
  'UPLOADED',
  'PROCESSING',
  'ANALYZED',
  'FAILED'
);

CREATE TYPE job_status AS ENUM (
  'QUEUED',
  'PROCESSING',
  'COMPLETED',
  'FAILED',
  'CANCELLED'
);

CREATE TYPE run_status AS ENUM (
  'QUEUED',
  'PROCESSING',
  'NORMALIZING',
  'COMPLETED',
  'FAILED',
  'CANCELLED'
);

CREATE TABLE organizations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO organizations (id, name)
VALUES ('00000000-0000-0000-0000-000000000001', 'Local Development')
ON CONFLICT DO NOTHING;

CREATE TABLE recordings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id),
  original_name text NOT NULL,
  mime_type text NOT NULL CHECK (mime_type IN ('video/webm', 'video/mp4')),
  size_bytes bigint NOT NULL CHECK (size_bytes > 0),
  duration_sec double precision,
  status recording_status NOT NULL DEFAULT 'PENDING_UPLOAD',
  source text NOT NULL DEFAULT 'upload',
  object_key text NOT NULL UNIQUE,
  checksum_sha256 text,
  created_by text,
  updated_by text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX recordings_organization_created_idx
  ON recordings (organization_id, created_at DESC);

CREATE TABLE analysis_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id),
  recording_id uuid NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  provider text NOT NULL,
  model text,
  prompt_version text NOT NULL,
  normalization_version text NOT NULL,
  grouping_version text NOT NULL,
  status run_status NOT NULL DEFAULT 'QUEUED',
  raw_text text,
  error text,
  input_tokens bigint,
  output_tokens bigint,
  thinking_tokens bigint,
  total_tokens bigint,
  estimated_cost_usd numeric(14, 8),
  pricing_version text,
  started_at timestamptz,
  completed_at timestamptz,
  created_by text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX analysis_runs_recording_created_idx
  ON analysis_runs (recording_id, created_at DESC);

CREATE TABLE analysis_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id),
  recording_id uuid NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  analysis_run_id uuid NOT NULL UNIQUE REFERENCES analysis_runs(id) ON DELETE CASCADE,
  status job_status NOT NULL DEFAULT 'QUEUED',
  progress smallint NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  idempotency_key text NOT NULL UNIQUE,
  queue_task_id text,
  attempt_count integer NOT NULL DEFAULT 0,
  last_error text,
  locked_at timestamptz,
  started_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX analysis_jobs_status_created_idx
  ON analysis_jobs (status, created_at);

CREATE TABLE job_attempts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  analysis_job_id uuid NOT NULL REFERENCES analysis_jobs(id) ON DELETE CASCADE,
  attempt integer NOT NULL,
  status job_status NOT NULL,
  worker_id text,
  error text,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE (analysis_job_id, attempt)
);

CREATE TABLE outbox_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  aggregate_type text NOT NULL,
  aggregate_id uuid NOT NULL,
  event_type text NOT NULL,
  payload jsonb NOT NULL,
  attempts integer NOT NULL DEFAULT 0,
  available_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz,
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbox_unpublished_idx
  ON outbox_events (available_at, created_at)
  WHERE published_at IS NULL;

CREATE TABLE raw_vision_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  analysis_run_id uuid NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
  timestamp_ms integer NOT NULL CHECK (timestamp_ms >= 0),
  screen text NOT NULL,
  visible_text text,
  target text,
  event_type_guess text NOT NULL,
  color_cues jsonb NOT NULL DEFAULT '[]'::jsonb,
  state_change text,
  confidence double precision NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX raw_events_run_timestamp_idx
  ON raw_vision_events (analysis_run_id, timestamp_ms);

CREATE TABLE action_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  recording_id uuid NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  analysis_run_id uuid REFERENCES analysis_runs(id) ON DELETE SET NULL,
  timestamp_ms integer NOT NULL CHECK (timestamp_ms >= 0),
  canonical_action text NOT NULL,
  event_type text NOT NULL,
  screen text NOT NULL,
  entity_type text,
  entity_id text,
  order_id text,
  issue_type text,
  target text NOT NULL,
  confidence double precision NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  source_raw_event_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
  quality_flags jsonb NOT NULL DEFAULT '[]'::jsonb,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  source text NOT NULL DEFAULT 'gemini',
  qa_status text,
  qa_comment text,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX action_events_recording_timestamp_idx
  ON action_events (recording_id, timestamp_ms);

CREATE TABLE scenario_templates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id),
  code text,
  name text NOT NULL,
  issue_type text NOT NULL,
  signature text NOT NULL,
  frequency integer NOT NULL DEFAULT 0,
  average_duration_ms integer NOT NULL DEFAULT 0,
  median_duration_ms integer NOT NULL DEFAULT 0,
  p95_duration_ms integer NOT NULL DEFAULT 0,
  manual_check_count integer NOT NULL DEFAULT 0,
  repeated_action_count integer NOT NULL DEFAULT 0,
  confidence_average double precision NOT NULL DEFAULT 0,
  ambiguous_count integer NOT NULL DEFAULT 0,
  automation_score double precision NOT NULL DEFAULT 0,
  action_sequence jsonb NOT NULL DEFAULT '[]'::jsonb,
  metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
  status text NOT NULL DEFAULT 'candidate',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, signature)
);

CREATE TABLE scenario_instances (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  recording_id uuid NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  analysis_run_id uuid REFERENCES analysis_runs(id) ON DELETE SET NULL,
  template_id uuid REFERENCES scenario_templates(id) ON DELETE SET NULL,
  known_scenario_code text,
  order_id text,
  entity_type text,
  entity_id text,
  issue_type text NOT NULL,
  started_at_ms integer NOT NULL,
  ended_at_ms integer NOT NULL,
  duration_ms integer NOT NULL,
  event_ids jsonb NOT NULL,
  outcome text NOT NULL,
  status text NOT NULL DEFAULT 'confirmed',
  confidence double precision NOT NULL,
  boundary_rule_version text,
  quality_flags jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE automation_candidates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  template_id uuid NOT NULL REFERENCES scenario_templates(id) ON DELETE CASCADE,
  title text NOT NULL,
  type text,
  rationale text NOT NULL,
  affected_steps jsonb NOT NULL,
  impact text NOT NULL,
  confidence double precision NOT NULL,
  score double precision NOT NULL DEFAULT 0,
  status text NOT NULL DEFAULT 'proposed',
  breakdown jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE data_quality_issues (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  recording_id uuid NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  analysis_run_id uuid REFERENCES analysis_runs(id) ON DELETE SET NULL,
  raw_vision_event_id uuid REFERENCES raw_vision_events(id) ON DELETE SET NULL,
  action_event_id uuid REFERENCES action_events(id) ON DELETE SET NULL,
  type text NOT NULL,
  severity text NOT NULL,
  message text NOT NULL,
  timestamp_ms integer NOT NULL,
  resolved boolean NOT NULL DEFAULT false,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE analyst_reports (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  recording_id uuid NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  analysis_run_id uuid REFERENCES analysis_runs(id) ON DELETE SET NULL,
  summary text NOT NULL,
  observations jsonb NOT NULL,
  recommendations jsonb NOT NULL,
  metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
  graph_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  model text NOT NULL,
  provider text,
  prompt_version text,
  normalization_version text,
  grouping_version text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE scenario_graphs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  recording_id uuid NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
  analysis_run_id uuid REFERENCES analysis_runs(id) ON DELETE SET NULL,
  graph jsonb NOT NULL,
  metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE known_scenarios (
  organization_id uuid NOT NULL REFERENCES organizations(id),
  code text NOT NULL,
  name text NOT NULL,
  issue_type text NOT NULL,
  entity_type text,
  start_actions jsonb NOT NULL,
  required_actions jsonb NOT NULL,
  optional_actions jsonb NOT NULL,
  end_actions jsonb NOT NULL,
  forbidden_actions jsonb NOT NULL,
  timeout_ms integer NOT NULL,
  version text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, code)
);

CREATE TABLE boundary_rules (
  organization_id uuid NOT NULL REFERENCES organizations(id),
  id text NOT NULL,
  name text NOT NULL,
  priority integer NOT NULL,
  type text NOT NULL,
  conditions jsonb NOT NULL,
  version text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, id)
);

CREATE TABLE ground_truth_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id),
  recording_id uuid REFERENCES recordings(id) ON DELETE CASCADE,
  timestamp_ms integer NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE settings_audit_log (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id),
  actor_id text,
  entity_type text NOT NULL,
  entity_id text NOT NULL,
  before_value jsonb,
  after_value jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO known_scenarios (
  organization_id, code, name, issue_type, entity_type, start_actions,
  required_actions, optional_actions, end_actions, forbidden_actions,
  timeout_ms, version
) VALUES
(
  '00000000-0000-0000-0000-000000000001', 'LATE_PICKUP',
  'Опоздание на забор', 'Late pickup', 'order',
  '["OPEN_ORDER","TAKE_ACTION","CHECK"]',
  '["CHECK"]', '["EDIT_FIELD","SAVE"]',
  '["MARK_PICKUP_COMPLETED","RESOLVE_ISSUE"]', '[]',
  1200000, 'known-scenarios-v1'
),
(
  '00000000-0000-0000-0000-000000000001', 'UNASSIGNED_COURIER',
  'Курьер не назначен', 'Unassigned courier', 'order',
  '["OPEN_ORDER","TAKE_ACTION"]',
  '["ASSIGN_DRIVER"]', '["OPEN_DRIVER_ASSIGNMENT"]',
  '["RESOLVE_ISSUE"]', '[]',
  1200000, 'known-scenarios-v1'
)
ON CONFLICT DO NOTHING;

INSERT INTO boundary_rules (
  organization_id, id, name, priority, type, conditions, version
) VALUES (
  '00000000-0000-0000-0000-000000000001',
  'split-on-entity-change',
  'Разделять сценарии при смене сущности',
  100,
  'entity_change',
  '{"requireDifferentEntityId":true}',
  'boundary-rules-v2'
)
ON CONFLICT DO NOTHING;
