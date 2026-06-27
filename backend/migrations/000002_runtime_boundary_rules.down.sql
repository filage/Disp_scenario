DELETE FROM boundary_rules
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND id IN (
    'start-issue-work',
    'end-resolution-action',
    'end-save-after-required',
    'end-timeout'
  );

UPDATE boundary_rules
SET name = 'Разделять сценарии при смене сущности',
    priority = 100,
    type = 'entity_change',
    conditions = '{"requireDifferentEntityId":true}'::jsonb,
    version = 'boundary-rules-v2',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND id = 'split-on-entity-change';
