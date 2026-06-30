ALTER TABLE analysis_jobs
  ADD COLUMN requested_by text;

CREATE TABLE provider_credentials (
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  subject text NOT NULL,
  provider text NOT NULL,
  encrypted_secret bytea NOT NULL,
  nonce bytea NOT NULL,
  last_four text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, subject, provider)
);
