INSERT INTO known_scenarios (
  organization_id, code, name, issue_type, entity_type, start_actions,
  required_actions, optional_actions, end_actions, forbidden_actions,
  timeout_ms, version, enabled
) VALUES
(
  '00000000-0000-0000-0000-000000000001', 'CHANGE_DELIVERY_DESTINATION',
  'Смена точки окончания доставки', 'Delivery destination change', 'order',
  '["OPEN_ORDER","OPEN_FIELD_EDITOR"]', '["CHANGE_FIELD_VALUE"]',
  '["EDIT_FIELD"]', '["SAVE"]', '[]', 1200000, 'known-scenarios-v4', true
),
(
  '00000000-0000-0000-0000-000000000001', 'UPDATE_RECIPIENT_CONTACT',
  'Обновление контакта получателя', 'Recipient contact update', 'order',
  '["OPEN_ORDER","OPEN_FIELD_EDITOR"]', '["CHANGE_FIELD_VALUE"]',
  '["EDIT_FIELD"]', '["SAVE"]', '[]', 1200000, 'known-scenarios-v4', true
),
(
  '00000000-0000-0000-0000-000000000001', 'ADD_DELIVERY_NOTE',
  'Добавление комментария к доставке', 'Delivery note update', 'order',
  '["OPEN_ORDER","OPEN_FIELD_EDITOR"]', '["CHANGE_FIELD_VALUE"]',
  '["EDIT_FIELD"]', '["SAVE"]', '[]', 1200000, 'known-scenarios-v4', true
)
ON CONFLICT (organization_id, code) DO UPDATE SET
  name = EXCLUDED.name,
  issue_type = EXCLUDED.issue_type,
  entity_type = EXCLUDED.entity_type,
  start_actions = EXCLUDED.start_actions,
  required_actions = EXCLUDED.required_actions,
  optional_actions = EXCLUDED.optional_actions,
  end_actions = EXCLUDED.end_actions,
  forbidden_actions = EXCLUDED.forbidden_actions,
  timeout_ms = EXCLUDED.timeout_ms,
  version = EXCLUDED.version,
  enabled = EXCLUDED.enabled,
  updated_at = now();
