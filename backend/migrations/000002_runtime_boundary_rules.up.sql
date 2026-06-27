UPDATE boundary_rules
SET name = 'Разделение при смене сущности',
    priority = 70,
    type = 'split',
    conditions = '{"entityChange":true}'::jsonb,
    version = '1.0.0',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND id = 'split-on-entity-change';

INSERT INTO boundary_rules (
  organization_id, id, name, priority, type, conditions, version, enabled
) VALUES
(
  '00000000-0000-0000-0000-000000000001',
  'start-issue-work',
  'Старт работы с проблемным заказом',
  100,
  'start',
  '{"actions":["OPEN_ORDER","TAKE_ACTION","CHECK"],"requiresIssueType":true}',
  '1.0.0',
  true
),
(
  '00000000-0000-0000-0000-000000000001',
  'end-resolution-action',
  'Завершение по действию решения',
  100,
  'end',
  '{"actions":["RESOLVE_ISSUE","MARK_PICKUP_COMPLETED"]}',
  '1.0.0',
  true
),
(
  '00000000-0000-0000-0000-000000000001',
  'end-save-after-required',
  'Завершение по сохранению после обязательного шага',
  80,
  'end',
  '{"actions":["SAVE"],"requiresPriorActions":["EDIT_FIELD","ASSIGN_DRIVER"]}',
  '1.0.0',
  false
),
(
  '00000000-0000-0000-0000-000000000001',
  'end-timeout',
  'Завершение по таймауту активности',
  60,
  'end',
  '{"inactivityMs":600000}',
  '1.0.0',
  true
)
ON CONFLICT (organization_id, id) DO NOTHING;
