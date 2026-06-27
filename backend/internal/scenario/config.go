package scenario

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/dispscenario-analyst-v2/internal/domain"
	"github.com/example/dispscenario-analyst-v2/internal/platform"
)

func LoadConfig(ctx context.Context, pool *pgxpool.Pool) (domain.ScenarioConfig, error) {
	rows, err := pool.Query(ctx, `
		SELECT code,name,issue_type,entity_type,required_actions,end_actions,timeout_ms
		FROM known_scenarios
		WHERE organization_id=$1 AND enabled=true
		ORDER BY code`, platform.LocalOrganizationID)
	if err != nil {
		return domain.ScenarioConfig{}, err
	}
	defer rows.Close()
	known := []domain.KnownScenario{}
	for rows.Next() {
		var item domain.KnownScenario
		var entityType *string
		var requiredJSON, endJSON []byte
		if err := rows.Scan(
			&item.Code, &item.Name, &item.IssueType, &entityType,
			&requiredJSON, &endJSON, &item.TimeoutMS,
		); err != nil {
			return domain.ScenarioConfig{}, err
		}
		if entityType != nil {
			item.EntityType = *entityType
		}
		_ = json.Unmarshal(requiredJSON, &item.RequiredActions)
		_ = json.Unmarshal(endJSON, &item.EndActions)
		known = append(known, item)
	}
	if err := rows.Err(); err != nil {
		return domain.ScenarioConfig{}, err
	}
	config := domain.ScenarioConfig{
		KnownScenarios: known, SplitOnEntityChange: true,
		BoundaryRuleVersion: domain.BoundaryRuleVersion,
	}
	ruleRows, err := pool.Query(ctx, `
		SELECT id,priority,type,conditions,version
		FROM boundary_rules
		WHERE organization_id=$1 AND enabled=true
		ORDER BY priority DESC,id`, platform.LocalOrganizationID)
	if err != nil {
		return domain.ScenarioConfig{}, err
	}
	defer ruleRows.Close()
	for ruleRows.Next() {
		var rule domain.BoundaryRule
		var conditionsJSON []byte
		if err := ruleRows.Scan(
			&rule.ID, &rule.Priority, &rule.Type, &conditionsJSON, &rule.Version,
		); err != nil {
			return domain.ScenarioConfig{}, err
		}
		var conditions struct {
			Actions                  []string `json:"actions"`
			RequiresIssueType        bool     `json:"requiresIssueType"`
			RequiresPriorActions     []string `json:"requiresPriorActions"`
			EntityChange             bool     `json:"entityChange"`
			RequireDifferentEntityID bool     `json:"requireDifferentEntityId"`
			InactivityMS             int      `json:"inactivityMs"`
		}
		if err := json.Unmarshal(conditionsJSON, &conditions); err != nil {
			return domain.ScenarioConfig{}, err
		}
		rule.Actions = conditions.Actions
		rule.RequiresIssueType = conditions.RequiresIssueType
		rule.RequiresPriorActions = conditions.RequiresPriorActions
		rule.EntityChange = conditions.EntityChange
		rule.RequireDifferentEntityID = conditions.RequireDifferentEntityID
		rule.InactivityMS = conditions.InactivityMS
		config.BoundaryRules = append(config.BoundaryRules, rule)
		if rule.Version != "" {
			config.BoundaryRuleVersion = rule.Version
		}
	}
	if err := ruleRows.Err(); err != nil {
		return domain.ScenarioConfig{}, err
	}
	return config, nil
}
