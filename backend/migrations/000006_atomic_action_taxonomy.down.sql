UPDATE known_scenarios
SET required_actions = '["ASSIGN_DRIVER"]'::jsonb,
    optional_actions = '["OPEN_DRIVER_ASSIGNMENT"]'::jsonb,
    version = 'known-scenarios-v2',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND code = 'UNASSIGNED_COURIER';

UPDATE known_scenarios
SET optional_actions = '["EDIT_FIELD","SAVE"]'::jsonb,
    version = 'known-scenarios-v2',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND code = 'LATE_PICKUP';

UPDATE boundary_rules
SET conditions = '{"actions":["SAVE"],"requiresPriorActions":["EDIT_FIELD","ASSIGN_DRIVER"]}'::jsonb,
    version = '1.0.0',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND id = 'end-save-after-required';
