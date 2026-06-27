UPDATE known_scenarios
SET start_actions = CASE code
      WHEN 'LATE_PICKUP' THEN '["OPEN_ORDER","TAKE_ACTION","CHECK"]'::jsonb
      ELSE '["OPEN_ORDER","TAKE_ACTION"]'::jsonb
    END,
    end_actions = CASE code
      WHEN 'LATE_PICKUP' THEN '["MARK_PICKUP_COMPLETED","RESOLVE_ISSUE"]'::jsonb
      ELSE '["RESOLVE_ISSUE"]'::jsonb
    END,
    version = 'known-scenarios-v1',
    updated_at = now()
WHERE organization_id = '00000000-0000-0000-0000-000000000001'
  AND code IN ('LATE_PICKUP', 'UNASSIGNED_COURIER');
