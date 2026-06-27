UPDATE known_scenarios
SET required_actions = '["SEND_TO_SELECTED_DRIVER"]'::jsonb,
    optional_actions = '["OPEN_DRIVER_ASSIGNMENT","SELECT_DRIVER"]'::jsonb,
    version = 'known-scenarios-v3',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND code = 'UNASSIGNED_COURIER';

UPDATE known_scenarios
SET optional_actions = '["OPEN_FIELD_EDITOR","CHANGE_FIELD_VALUE","EDIT_FIELD","SAVE"]'::jsonb,
    version = 'known-scenarios-v3',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND code = 'LATE_PICKUP';

UPDATE boundary_rules
SET conditions = '{"actions":["SAVE"],"requiresPriorActions":["CHANGE_FIELD_VALUE","EDIT_FIELD","SEND_TO_SELECTED_DRIVER","ASSIGN_DRIVER"]}'::jsonb,
    version = '1.1.0',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND id = 'end-save-after-required';
