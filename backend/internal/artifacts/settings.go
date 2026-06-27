package artifacts

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/example/dispscenario-analyst-v2/internal/domain"
	"github.com/example/dispscenario-analyst-v2/internal/platform"
)

type KnownScenarioConfig struct {
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	IssueType        string          `json:"issueType"`
	EntityType       *string         `json:"entityType,omitempty"`
	StartActions     json.RawMessage `json:"startActions"`
	RequiredActions  json.RawMessage `json:"requiredActions"`
	OptionalActions  json.RawMessage `json:"optionalActions"`
	EndActions       json.RawMessage `json:"endActions"`
	ForbiddenActions json.RawMessage `json:"forbiddenActions"`
	TimeoutMS        int             `json:"timeoutMs"`
	Version          string          `json:"version"`
	Enabled          bool            `json:"enabled"`
}

type BoundaryRuleConfig struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Priority   int             `json:"priority"`
	Type       string          `json:"type"`
	Conditions json.RawMessage `json:"conditions"`
	Version    string          `json:"version"`
	Enabled    bool            `json:"enabled"`
}

func (s *Service) Settings(ctx context.Context) (map[string]any, error) {
	knownRows, err := s.pool.Query(ctx, `
		SELECT code, name, issue_type, entity_type, start_actions, required_actions,
		       optional_actions, end_actions, forbidden_actions, timeout_ms, version, enabled
		FROM known_scenarios
		WHERE organization_id = $1 ORDER BY code`, platform.LocalOrganizationID)
	if err != nil {
		return nil, err
	}
	known := []KnownScenarioConfig{}
	for knownRows.Next() {
		var item KnownScenarioConfig
		if err := knownRows.Scan(
			&item.Code, &item.Name, &item.IssueType, &item.EntityType,
			&item.StartActions, &item.RequiredActions, &item.OptionalActions,
			&item.EndActions, &item.ForbiddenActions, &item.TimeoutMS,
			&item.Version, &item.Enabled,
		); err != nil {
			knownRows.Close()
			return nil, err
		}
		known = append(known, item)
	}
	knownRows.Close()

	ruleRows, err := s.pool.Query(ctx, `
		SELECT id, name, priority, type, conditions, version, enabled
		FROM boundary_rules
		WHERE organization_id = $1 ORDER BY priority DESC, id`, platform.LocalOrganizationID)
	if err != nil {
		return nil, err
	}
	rules := []BoundaryRuleConfig{}
	for ruleRows.Next() {
		var item BoundaryRuleConfig
		if err := ruleRows.Scan(
			&item.ID, &item.Name, &item.Priority, &item.Type,
			&item.Conditions, &item.Version, &item.Enabled,
		); err != nil {
			ruleRows.Close()
			return nil, err
		}
		rules = append(rules, item)
	}
	ruleRows.Close()

	return map[string]any{
		"versions": map[string]string{
			"promptVersion":        "video-raw-extractor-v8",
			"normalizationVersion": domain.NormalizationVersion,
			"groupingVersion":      "scenario-grouping-v6",
		},
		"knownScenarios": known, "boundaryRules": rules,
		"actionCatalog": []string{
			"OPEN_ORDER", "FILTER_ISSUES", "CHECK", "INSPECT_ISSUE",
			"RESOLUTION_ATTEMPT", "TAKE_ACTION", "OPEN_DRIVER_ASSIGNMENT",
			"SELECT_DRIVER", "SEND_TO_SELECTED_DRIVER", "ASSIGN_DRIVER",
			"MARK_PICKUP_COMPLETED", "OPEN_FIELD_EDITOR", "CHANGE_FIELD_VALUE", "EDIT_FIELD",
			"SAVE", "RESOLVE_ISSUE", "NAVIGATE",
		},
		"dataQualityFlags": []string{
			"LOW_CONFIDENCE", "UNKNOWN_TARGET", "OUT_OF_ORDER_TIMESTAMP",
			"DUPLICATE_ACTION", "AMBIGUOUS_BOUNDARY", "ACTION_FAILED",
			"MISSING_SCENARIO_END", "GEMINI_PARSE_FALLBACK", "GEMINI_BOUNDARY_REVIEW",
		},
	}, nil
}

type KnownScenarioPatch struct {
	Name             *string          `json:"name"`
	IssueType        *string          `json:"issueType"`
	EntityType       *string          `json:"entityType"`
	StartActions     *json.RawMessage `json:"startActions"`
	RequiredActions  *json.RawMessage `json:"requiredActions"`
	OptionalActions  *json.RawMessage `json:"optionalActions"`
	EndActions       *json.RawMessage `json:"endActions"`
	ForbiddenActions *json.RawMessage `json:"forbiddenActions"`
	TimeoutMS        *int             `json:"timeoutMs"`
	Enabled          *bool            `json:"enabled"`
}

func (s *Service) PatchKnownScenario(
	ctx context.Context,
	code string,
	patch KnownScenarioPatch,
	actor string,
) (KnownScenarioConfig, error) {
	before, err := s.knownScenario(ctx, code)
	if err != nil {
		return KnownScenarioConfig{}, err
	}
	var item KnownScenarioConfig
	err = s.pool.QueryRow(ctx, `
		UPDATE known_scenarios SET
			name=COALESCE($3,name), issue_type=COALESCE($4,issue_type),
			entity_type=CASE WHEN $5::text IS NULL THEN entity_type ELSE NULLIF($5,'') END,
			start_actions=COALESCE($6,start_actions),
			required_actions=COALESCE($7,required_actions),
			optional_actions=COALESCE($8,optional_actions),
			end_actions=COALESCE($9,end_actions),
			forbidden_actions=COALESCE($10,forbidden_actions),
			timeout_ms=COALESCE($11,timeout_ms), enabled=COALESCE($12,enabled),
			updated_at=now()
		WHERE organization_id=$1 AND code=$2
		RETURNING code,name,issue_type,entity_type,start_actions,required_actions,
		          optional_actions,end_actions,forbidden_actions,timeout_ms,version,enabled`,
		platform.LocalOrganizationID, code, patch.Name, patch.IssueType, patch.EntityType,
		patch.StartActions, patch.RequiredActions, patch.OptionalActions, patch.EndActions,
		patch.ForbiddenActions, patch.TimeoutMS, patch.Enabled,
	).Scan(
		&item.Code, &item.Name, &item.IssueType, &item.EntityType,
		&item.StartActions, &item.RequiredActions, &item.OptionalActions,
		&item.EndActions, &item.ForbiddenActions, &item.TimeoutMS,
		&item.Version, &item.Enabled,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return KnownScenarioConfig{}, ErrNotFound
	}
	if err != nil {
		return KnownScenarioConfig{}, err
	}
	_ = s.audit(ctx, actor, "known_scenario", code, before, item)
	return item, nil
}

type BoundaryRulePatch struct {
	Name       *string          `json:"name"`
	Priority   *int             `json:"priority"`
	Type       *string          `json:"type"`
	Conditions *json.RawMessage `json:"conditions"`
	Enabled    *bool            `json:"enabled"`
}

func (s *Service) PatchBoundaryRule(
	ctx context.Context,
	id string,
	patch BoundaryRulePatch,
	actor string,
) (BoundaryRuleConfig, error) {
	before, err := s.boundaryRule(ctx, id)
	if err != nil {
		return BoundaryRuleConfig{}, err
	}
	var item BoundaryRuleConfig
	err = s.pool.QueryRow(ctx, `
		UPDATE boundary_rules SET
			name=COALESCE($3,name), priority=COALESCE($4,priority),
			type=COALESCE($5,type), conditions=COALESCE($6,conditions),
			enabled=COALESCE($7,enabled), updated_at=now()
		WHERE organization_id=$1 AND id=$2
		RETURNING id,name,priority,type,conditions,version,enabled`,
		platform.LocalOrganizationID, id, patch.Name, patch.Priority,
		patch.Type, patch.Conditions, patch.Enabled,
	).Scan(&item.ID, &item.Name, &item.Priority, &item.Type, &item.Conditions, &item.Version, &item.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return BoundaryRuleConfig{}, ErrNotFound
	}
	if err != nil {
		return BoundaryRuleConfig{}, err
	}
	_ = s.audit(ctx, actor, "boundary_rule", id, before, item)
	return item, nil
}

func (s *Service) knownScenario(ctx context.Context, code string) (KnownScenarioConfig, error) {
	var item KnownScenarioConfig
	err := s.pool.QueryRow(ctx, `
		SELECT code,name,issue_type,entity_type,start_actions,required_actions,
		       optional_actions,end_actions,forbidden_actions,timeout_ms,version,enabled
		FROM known_scenarios WHERE organization_id=$1 AND code=$2`,
		platform.LocalOrganizationID, code,
	).Scan(
		&item.Code, &item.Name, &item.IssueType, &item.EntityType,
		&item.StartActions, &item.RequiredActions, &item.OptionalActions,
		&item.EndActions, &item.ForbiddenActions, &item.TimeoutMS, &item.Version, &item.Enabled,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (s *Service) boundaryRule(ctx context.Context, id string) (BoundaryRuleConfig, error) {
	var item BoundaryRuleConfig
	err := s.pool.QueryRow(ctx, `
		SELECT id,name,priority,type,conditions,version,enabled
		FROM boundary_rules WHERE organization_id=$1 AND id=$2`,
		platform.LocalOrganizationID, id,
	).Scan(&item.ID, &item.Name, &item.Priority, &item.Type, &item.Conditions, &item.Version, &item.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (s *Service) audit(ctx context.Context, actor, entityType, entityID string, before, after any) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO settings_audit_log (
			organization_id, actor_id, entity_type, entity_id, before_value, after_value
		) VALUES ($1,$2,$3,$4,$5,$6)`,
		platform.LocalOrganizationID, actor, entityType, entityID, beforeJSON, afterJSON)
	return err
}
