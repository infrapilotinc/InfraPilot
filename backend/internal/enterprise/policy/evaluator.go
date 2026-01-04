package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Action constants for policy evaluation
const (
	ActionBlock = "block"
	ActionWarn  = "warn"
	ActionAudit = "audit"
)

// Condition represents a single policy condition
type Condition struct {
	Check    string      `json:"check"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// EvaluationResult represents the result of evaluating a policy
type EvaluationResult struct {
	PolicyID   uuid.UUID `json:"policy_id"`
	PolicyName string    `json:"policy_name"`
	Violated   bool      `json:"violated"`
	Action     string    `json:"action"`
	Message    string    `json:"message"`
}

// Resource represents a resource to evaluate policies against
type Resource struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id"`
	Attributes map[string]interface{} `json:"attributes"`
}

// Evaluator handles policy evaluation
type Evaluator struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

// NewEvaluator creates a new policy evaluator
func NewEvaluator(db *pgxpool.Pool, logger *zap.Logger) *Evaluator {
	return &Evaluator{
		db:     db,
		logger: logger,
	}
}

// EvaluateResource evaluates all applicable policies for a resource
func (e *Evaluator) EvaluateResource(ctx context.Context, orgID uuid.UUID, resource Resource) ([]EvaluationResult, error) {
	// Get all enabled policies for this org and resource type
	policies, err := e.getPoliciesForType(ctx, orgID, resource.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to get policies: %w", err)
	}

	var results []EvaluationResult
	for _, policy := range policies {
		result := e.evaluatePolicy(policy, resource)
		results = append(results, result)

		// Record violation if policy was violated
		if result.Violated {
			if err := e.recordViolation(ctx, policy, resource, result.Message); err != nil {
				e.logger.Error("Failed to record violation", zap.Error(err))
			}
		}
	}

	return results, nil
}

// EvaluateAndBlock evaluates policies and returns true if action should be blocked
func (e *Evaluator) EvaluateAndBlock(ctx context.Context, orgID uuid.UUID, resource Resource) (bool, string, error) {
	results, err := e.EvaluateResource(ctx, orgID, resource)
	if err != nil {
		return false, "", err
	}

	for _, result := range results {
		if result.Violated && result.Action == ActionBlock {
			return true, result.Message, nil
		}
	}

	return false, "", nil
}

// getPoliciesForType retrieves all enabled policies for a resource type
func (e *Evaluator) getPoliciesForType(ctx context.Context, orgID uuid.UUID, policyType string) ([]Policy, error) {
	query := `
		SELECT id, org_id, name, description, policy_type, conditions, action, applies_to, enabled, priority, created_by, created_at, updated_at
		FROM policies
		WHERE org_id = $1 AND policy_type = $2 AND enabled = true
		ORDER BY priority DESC
	`

	rows, err := e.db.Query(ctx, query, orgID, policyType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var p Policy
		err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.Description, &p.PolicyType,
			&p.Conditions, &p.Action, &p.AppliesTo,
			&p.Enabled, &p.Priority, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}

	return policies, rows.Err()
}

// evaluatePolicy evaluates a single policy against a resource
func (e *Evaluator) evaluatePolicy(policy Policy, resource Resource) EvaluationResult {
	result := EvaluationResult{
		PolicyID:   policy.ID,
		PolicyName: policy.Name,
		Action:     policy.Action,
		Violated:   false,
	}

	// Check if policy applies to this resource
	if !e.policyApplies(policy, resource) {
		return result
	}

	// Evaluate conditions
	violated, message := e.evaluateConditions(policy.Conditions, resource.Attributes)
	result.Violated = violated
	result.Message = message

	return result
}

// policyApplies checks if a policy applies to a resource
func (e *Evaluator) policyApplies(policy Policy, resource Resource) bool {
	if len(policy.AppliesTo) == 0 {
		return true // No restrictions, applies to all
	}

	// Check labels
	if labels, ok := policy.AppliesTo["labels"].(map[string]interface{}); ok {
		resourceLabels, _ := resource.Attributes["labels"].(map[string]interface{})
		for key, value := range labels {
			if resourceLabels[key] != value {
				return false
			}
		}
	}

	// Check agent
	if agentID, ok := policy.AppliesTo["agent_id"].(string); ok {
		if resource.Attributes["agent_id"] != agentID {
			return false
		}
	}

	return true
}

// evaluateConditions evaluates policy conditions against resource attributes
func (e *Evaluator) evaluateConditions(conditions map[string]interface{}, attributes map[string]interface{}) (bool, string) {
	check, _ := conditions["check"].(string)
	operator, _ := conditions["operator"].(string)
	expectedValue := conditions["value"]

	if check == "" || operator == "" {
		return false, ""
	}

	// Get actual value from attributes
	actualValue := getNestedValue(attributes, check)

	// Evaluate condition
	violated := e.evaluateOperator(operator, actualValue, expectedValue)

	if violated {
		return true, fmt.Sprintf("Policy violation: %s %s %v (actual: %v)", check, operator, expectedValue, actualValue)
	}

	return false, ""
}

// evaluateOperator evaluates a condition operator
func (e *Evaluator) evaluateOperator(operator string, actual, expected interface{}) bool {
	switch operator {
	case "equals", "eq", "==":
		return actual == expected
	case "not_equals", "neq", "!=":
		return actual != expected
	case "contains":
		actualStr, ok1 := actual.(string)
		expectedStr, ok2 := expected.(string)
		if ok1 && ok2 {
			return strings.Contains(actualStr, expectedStr)
		}
		return false
	case "not_contains":
		actualStr, ok1 := actual.(string)
		expectedStr, ok2 := expected.(string)
		if ok1 && ok2 {
			return !strings.Contains(actualStr, expectedStr)
		}
		return true
	case "greater_than", "gt", ">":
		return compareNumbers(actual, expected) > 0
	case "less_than", "lt", "<":
		return compareNumbers(actual, expected) < 0
	case "in":
		if arr, ok := expected.([]interface{}); ok {
			for _, v := range arr {
				if v == actual {
					return true
				}
			}
		}
		return false
	case "not_in":
		if arr, ok := expected.([]interface{}); ok {
			for _, v := range arr {
				if v == actual {
					return false
				}
			}
		}
		return true
	case "exists":
		return actual != nil
	case "not_exists":
		return actual == nil
	case "matches":
		// Simple pattern matching with wildcards
		actualStr, ok1 := actual.(string)
		expectedStr, ok2 := expected.(string)
		if ok1 && ok2 {
			return matchPattern(actualStr, expectedStr)
		}
		return false
	default:
		return false
	}
}

// recordViolation records a policy violation
func (e *Evaluator) recordViolation(ctx context.Context, policy Policy, resource Resource, message string) error {
	query := `
		INSERT INTO policy_violations (policy_id, resource_type, resource_id, message, context)
		VALUES ($1, $2, $3, $4, $5)
	`

	contextData, _ := json.Marshal(resource.Attributes)

	_, err := e.db.Exec(ctx, query,
		policy.ID,
		resource.Type,
		resource.ID,
		message,
		contextData,
	)

	return err
}

// getNestedValue retrieves a nested value from a map using dot notation
func getNestedValue(m map[string]interface{}, key string) interface{} {
	parts := strings.Split(key, ".")
	current := interface{}(m)

	for _, part := range parts {
		if currentMap, ok := current.(map[string]interface{}); ok {
			current = currentMap[part]
		} else {
			return nil
		}
	}

	return current
}

// compareNumbers compares two numeric values
func compareNumbers(a, b interface{}) int {
	aFloat := toFloat64(a)
	bFloat := toFloat64(b)

	if aFloat < bFloat {
		return -1
	} else if aFloat > bFloat {
		return 1
	}
	return 0
}

// toFloat64 converts an interface to float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	default:
		return 0
	}
}

// matchPattern performs simple wildcard pattern matching
func matchPattern(s, pattern string) bool {
	// Simple * wildcard support
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(s, pattern[1:len(pattern)-1])
	}

	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(s, pattern[1:])
	}

	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(s, pattern[:len(pattern)-1])
	}

	return s == pattern
}

// ContainerPolicy helpers for common container policy checks

// CheckContainerRootUser checks if a container is running as root
func CheckContainerRootUser(user string) Resource {
	return Resource{
		Type: "container",
		Attributes: map[string]interface{}{
			"user": user,
		},
	}
}

// CheckContainerPrivileged checks if a container is running in privileged mode
func CheckContainerPrivileged(privileged bool) Resource {
	return Resource{
		Type: "container",
		Attributes: map[string]interface{}{
			"privileged": privileged,
		},
	}
}

// CheckContainerPorts checks container port bindings
func CheckContainerPorts(ports []string) Resource {
	return Resource{
		Type: "container",
		Attributes: map[string]interface{}{
			"ports": ports,
		},
	}
}

// ProxyPolicy helpers for common proxy policy checks

// CheckProxySSL checks if a proxy has SSL enabled
func CheckProxySSL(sslEnabled bool, domains []string) Resource {
	return Resource{
		Type: "proxy",
		Attributes: map[string]interface{}{
			"ssl_enabled": sslEnabled,
			"domains":     domains,
		},
	}
}

// CheckProxyRateLimit checks if a proxy has rate limiting
func CheckProxyRateLimit(hasRateLimit bool) Resource {
	return Resource{
		Type: "proxy",
		Attributes: map[string]interface{}{
			"rate_limit_enabled": hasRateLimit,
		},
	}
}
