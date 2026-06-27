UPDATE known_scenarios
SET start_actions = '["OPEN_ORDER"]'::jsonb,
    end_actions = '["RESOLVE_ISSUE"]'::jsonb,
    version = 'known-scenarios-v2',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND code IN ('LATE_PICKUP', 'UNASSIGNED_COURIER');
