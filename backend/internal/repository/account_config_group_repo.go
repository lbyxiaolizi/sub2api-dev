package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var _ service.AccountConfigGroupRepository = (*accountRepository)(nil)

const accountConfigGroupColumns = `cg.id, cg.name, cg.group_id, cg.platform, cg.type, cg.config, cg.created_at, cg.updated_at`

func readAccountConfigGroups(ctx context.Context, exec sqlExecutor, where string, args ...any) ([]service.AccountConfigGroup, error) {
	rows, err := exec.QueryContext(ctx, `SELECT `+accountConfigGroupColumns+`
        FROM account_config_groups cg JOIN groups g ON g.id = cg.group_id AND g.deleted_at IS NULL `+where+` ORDER BY cg.id`, args...)
	if err != nil {
		return nil, err
	}
	groups := make([]service.AccountConfigGroup, 0)
	for rows.Next() {
		var group service.AccountConfigGroup
		var config []byte
		if err := rows.Scan(&group.ID, &group.Name, &group.GroupID, &group.Platform, &group.Type, &config, &group.CreatedAt, &group.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(config, &group.Config); err != nil {
			_ = rows.Close()
			return nil, err
		}
		group.AccountIDs = []int64{}
		groups = append(groups, group)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}
	ids := make([]int64, len(groups))
	index := make(map[int64]int, len(groups))
	for i := range groups {
		ids[i] = groups[i].ID
		index[groups[i].ID] = i
	}
	rows, err = exec.QueryContext(ctx, `SELECT m.account_config_group_id, m.account_id
        FROM account_config_group_members m JOIN accounts a ON a.id = m.account_id AND a.deleted_at IS NULL
        WHERE m.account_config_group_id = ANY($1) ORDER BY m.account_id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, accountID int64
		if err := rows.Scan(&id, &accountID); err != nil {
			return nil, err
		}
		groups[index[id]].AccountIDs = append(groups[index[id]].AccountIDs, accountID)
	}
	return groups, rows.Err()
}

func (r *accountRepository) ListAccountConfigGroups(ctx context.Context, parentGroupID int64) ([]service.AccountConfigGroup, error) {
	if parentGroupID > 0 {
		return readAccountConfigGroups(ctx, r.client, "WHERE cg.group_id = $1", parentGroupID)
	}
	return readAccountConfigGroups(ctx, r.client, "")
}

func (r *accountRepository) GetAccountConfigGroup(ctx context.Context, id int64) (*service.AccountConfigGroup, error) {
	groups, err := readAccountConfigGroups(ctx, r.client, "WHERE cg.id = $1", id)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, service.ErrAccountConfigGroupNotFound
	}
	return &groups[0], nil
}

func (r *accountRepository) GetAccountConfigGroupByAccount(ctx context.Context, accountID int64) (*service.AccountConfigGroup, error) {
	groups, err := readAccountConfigGroups(ctx, r.client, `WHERE cg.id = (SELECT account_config_group_id FROM account_config_group_members WHERE account_id = $1)`, accountID)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil
	}
	return &groups[0], nil
}

// Account locks serialize membership changes even between different configuration
// groups. The membership primary key and deferred routing FK are the final guard.
func (r *accountRepository) SaveAccountConfigGroup(ctx context.Context, group *service.AccountConfigGroup) error {
	if group == nil {
		return service.ErrAccountConfigGroupInvalid
	}
	// Confirmation authorizes this attempt only. A reused in-memory group must
	// not silently authorize overwrites for members added by a later request.
	defer func() { group.ConfirmModelMappingOverwrite = false }()
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	client := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		client = tx.Client()
	}
	if err := lockLiveGroups(ctx, client, []int64{group.GroupID}); err != nil {
		return err
	}

	// Config ownership is immutable: moving a group must be explicit member removal
	// followed by creation under the other parent, never a silent cross-group move.
	var previousConfig service.AccountConfigGroupConfig
	if group.ID != 0 {
		rows, err := client.QueryContext(ctx, `SELECT group_id, platform, type, updated_at, config FROM account_config_groups WHERE id = $1 FOR UPDATE`, group.ID)
		if err != nil {
			return err
		}
		var parent int64
		var platform, accountType string
		var updatedAt time.Time
		var previousConfigJSON []byte
		if !rows.Next() {
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return err
			}
			return service.ErrAccountConfigGroupNotFound
		}
		err = rows.Scan(&parent, &platform, &accountType, &updatedAt, &previousConfigJSON)
		_ = rows.Close()
		if err != nil {
			return err
		}
		if parent != group.GroupID || platform != group.Platform || accountType != group.Type {
			return service.ErrAccountConfigGroupInvalid
		}
		if !group.UpdatedAt.Equal(updatedAt) {
			return service.ErrAccountConfigGroupStale
		}
		if err := json.Unmarshal(previousConfigJSON, &previousConfig); err != nil {
			return err
		}
	}
	desiredIDs := append([]int64(nil), group.AccountIDs...)
	sort.Slice(desiredIDs, func(i, j int) bool { return desiredIDs[i] < desiredIDs[j] })
	for i, id := range desiredIDs {
		if id <= 0 || (i > 0 && desiredIDs[i-1] == id) {
			return service.ErrAccountConfigGroupInvalid
		}
	}
	oldIDs, err := accountConfigGroupMemberIDs(ctx, client, group.ID)
	if err != nil {
		return err
	}
	oldMembers := make(map[int64]bool, len(oldIDs))
	for _, id := range oldIDs {
		oldMembers[id] = true
	}
	lockIDs := mergeGroupIDs(oldIDs, desiredIDs)
	sort.Slice(lockIDs, func(i, j int) bool { return lockIDs[i] < lockIDs[j] })
	desired := make(map[int64]bool, len(desiredIDs))
	for _, id := range desiredIDs {
		desired[id] = true
	}

	type member struct {
		id                 int64
		credentials, extra map[string]any
	}
	members := make([]member, 0, len(desiredIDs))
	if len(lockIDs) > 0 {
		rows, err := client.QueryContext(ctx, `SELECT a.id, a.platform, a.type, a.parent_account_id, a.credentials, a.extra,
            EXISTS (SELECT 1 FROM account_groups ag WHERE ag.account_id = a.id AND ag.group_id = $2)
            FROM accounts a WHERE a.id = ANY($1) AND a.deleted_at IS NULL ORDER BY a.id FOR UPDATE OF a`, pq.Array(lockIDs), group.GroupID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var m member
			var platform, accountType string
			var parent sql.NullInt64
			var credentials, extra []byte
			var bound bool
			if err := rows.Scan(&m.id, &platform, &accountType, &parent, &credentials, &extra, &bound); err != nil {
				_ = rows.Close()
				return err
			}
			if !desired[m.id] {
				continue
			}
			if !bound || parent.Valid || platform != group.Platform || accountType != group.Type {
				_ = rows.Close()
				return service.ErrAccountConfigGroupInvalid
			}
			if err := json.Unmarshal(credentials, &m.credentials); err != nil {
				_ = rows.Close()
				return err
			}
			if err := json.Unmarshal(extra, &m.extra); err != nil {
				_ = rows.Close()
				return err
			}
			if enabled, _ := m.extra[service.UpstreamBillingRateSyncEnabledExtraKey].(bool); enabled {
				_ = rows.Close()
				return service.ErrUpstreamBillingRateSyncConflict
			}
			members = append(members, m)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		if len(members) != len(desiredIDs) {
			return service.ErrAccountConfigGroupInvalid
		}
		rows, err = client.QueryContext(ctx, `SELECT account_id FROM account_config_group_members
            WHERE account_id = ANY($1) AND account_config_group_id <> $2 LIMIT 1`, pq.Array(desiredIDs), group.ID)
		if err != nil {
			return err
		}
		conflict := rows.Next()
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		if conflict {
			return service.ErrAccountConfigGroupConflict
		}
	}
	if !group.ConfirmModelMappingOverwrite {
		for _, m := range members {
			if !oldMembers[m.id] {
				if err := validateAccountConfigGroupModelMappings(m.id, m.credentials, group.Config.Credentials); err != nil {
					return err
				}
			}
		}
	}

	config, err := json.Marshal(group.Config)
	if err != nil {
		return err
	}
	query := `INSERT INTO account_config_groups (name, group_id, platform, type, config) VALUES ($1,$2,$3,$4,$5)
        RETURNING id, created_at, updated_at`
	args := []any{group.Name, group.GroupID, group.Platform, group.Type, config}
	if group.ID != 0 {
		query = `UPDATE account_config_groups SET name=$1, config=$5, updated_at=clock_timestamp()
            WHERE id=$6 AND group_id=$2 AND platform=$3 AND type=$4 RETURNING id,created_at,updated_at`
		args = append(args, group.ID)
	}
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if !rows.Next() {
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		return service.ErrAccountConfigGroupNotFound
	}
	err = rows.Scan(&group.ID, &group.CreatedAt, &group.UpdatedAt)
	_ = rows.Close()
	if err != nil {
		return err
	}
	if _, err := client.ExecContext(ctx, `DELETE FROM account_config_group_members WHERE account_config_group_id=$1`, group.ID); err != nil {
		return err
	}
	for _, id := range oldIDs {
		if !desired[id] {
			if _, err := client.ExecContext(ctx, `UPDATE accounts SET credentials=credentials-$2, updated_at=NOW() WHERE id=$1 AND credentials ? $2`, id, service.AccountConfigGroupBaseURLOverrideKey); err != nil {
				return err
			}
		}
	}
	for _, m := range members {
		if _, err := client.ExecContext(ctx, `INSERT INTO account_config_group_members (account_id,account_config_group_id,group_id) VALUES ($1,$2,$3)`, m.id, group.ID, group.GroupID); err != nil {
			if isUniqueViolation(err) {
				return service.ErrAccountConfigGroupConflict
			}
			return err
		}
		credentials := replaceAccountConfigGroupCredentials(m.credentials, previousConfig.Credentials, group.Config.Credentials)
		extra := service.PrepareAccountConfigGroupMemberExtra(&service.Account{
			Platform: group.Platform, Type: group.Type, Extra: m.extra,
		}, group.Config.Extra)
		// Usage sessions/snapshots are bound to the upstream endpoint identity,
		// not just to the secret key. A group-wide endpoint switch invalidates them.
		if !reflect.DeepEqual(m.credentials["base_url"], credentials["base_url"]) || !reflect.DeepEqual(m.credentials["account_mode"], credentials["account_mode"]) {
			for _, key := range []string{"upstream_billing_probe", "ollama_cloud_usage_session", "ollama_cloud_usage_auto_refresh", "ollama_cloud_usage_snapshot", "opencode_go_usage_auto_refresh", "opencode_go_usage_snapshot"} {
				delete(extra, key)
			}
		}
		credentialJSON, err := json.Marshal(credentials)
		if err != nil {
			return err
		}
		extraJSON, err := json.Marshal(extra)
		if err != nil {
			return err
		}
		var expiresAt *time.Time
		if group.Config.ExpiresAt != nil && *group.Config.ExpiresAt > 0 {
			v := time.Unix(*group.Config.ExpiresAt, 0)
			expiresAt = &v
		}
		_, err = client.ExecContext(ctx, `UPDATE accounts SET notes=$2,
            proxy_id=CASE
                WHEN $4::bigint IS NULL THEN $3::bigint
                WHEN pool_id IS NOT DISTINCT FROM $4::bigint AND EXISTS (
                    SELECT 1 FROM proxies p WHERE p.id=accounts.proxy_id AND p.pool_id=$4 AND p.deleted_at IS NULL
                ) THEN proxy_id
                ELSE (SELECT p.id FROM proxies p
                    WHERE p.pool_id=$4 AND p.deleted_at IS NULL AND p.status='active' AND p.pool_health='healthy'
                    ORDER BY (SELECT count(*) FROM accounts assigned WHERE assigned.proxy_id=p.id AND assigned.deleted_at IS NULL), p.id
                    LIMIT 1)
            END, pool_id=$4,
            concurrency=$5, priority=$6, rate_multiplier=$7, load_factor=$8,
            status=CASE WHEN $16 OR $9 <> 'active' THEN $9 ELSE status END,
            schedulable=CASE WHEN $17 OR NOT $10 THEN $10 ELSE schedulable END,
            expires_at=$11, auto_pause_on_expired=$12,
            disable_auto_temp_unschedulable=$13, credentials=$14,
            extra=CASE WHEN credentials IS DISTINCT FROM $14::jsonb OR pool_id IS DISTINCT FROM $4
                    OR ($4::bigint IS NULL AND proxy_id IS DISTINCT FROM $3)
                THEN $15::jsonb - 'upstream_billing_probe' - 'ollama_cloud_usage_snapshot' - 'opencode_go_usage_snapshot'
                ELSE $15::jsonb END,
            temp_unschedulable_until=CASE WHEN $13 THEN NULL ELSE temp_unschedulable_until END,
            temp_unschedulable_reason=CASE WHEN $13 THEN NULL ELSE temp_unschedulable_reason END,
            proxy_fallback_origin_id=CASE WHEN proxy_id IS DISTINCT FROM $3 OR pool_id IS DISTINCT FROM $4 THEN NULL ELSE proxy_fallback_origin_id END,
            updated_at=NOW() WHERE id=$1`, m.id, group.Config.Notes, group.Config.ProxyID, group.Config.PoolID,
			group.Config.Concurrency, group.Config.Priority, group.Config.RateMultiplier, group.Config.LoadFactor,
			group.Config.Status, group.Config.Schedulable, expiresAt, group.Config.AutoPauseOnExpired,
			group.Config.DisableAutoTempUnschedulable, credentialJSON, extraJSON,
			!oldMembers[m.id] || previousConfig.Status != group.Config.Status,
			!oldMembers[m.id] || previousConfig.Schedulable != group.Config.Schedulable)
		if err != nil {
			return err
		}
	}
	for _, id := range lockIDs {
		accountID := id
		if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
			return err
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
		for _, id := range lockIDs {
			r.syncSchedulerAccountSnapshot(ctx, id)
		}
	}
	return nil
}

// Membership changes must not silently discard a newly joined account's model
// routes. Run this against the locked account snapshot, not a list projection or
// the earlier service-layer read. Deliberate edits to existing group members are
// unaffected; explicit membership confirmation still applies the exact group
// settings rather than merging incompatible mapping policies.
func validateAccountConfigGroupModelMappings(accountID int64, existing, replacement map[string]any) error {
	for _, field := range []string{"model_mapping", "compact_model_mapping"} {
		current := accountConfigGroupMappingEntries(existing[field])
		target := accountConfigGroupMappingEntries(replacement[field])
		removed, changed := 0, 0
		for key, value := range current {
			if next, exists := target[key]; !exists {
				removed++
			} else if !reflect.DeepEqual(value, next) {
				changed++
			}
		}
		if removed > 0 || changed > 0 {
			// No model names, mapping targets or credential values are exposed.
			return service.ErrAccountConfigGroupMappingConflict.WithMetadata(map[string]string{
				"account_id": strconv.FormatInt(accountID, 10), "field": field,
				"existing_entries": strconv.Itoa(len(current)), "target_entries": strconv.Itoa(len(target)),
				"removed_entries": strconv.Itoa(removed), "changed_entries": strconv.Itoa(changed),
			})
		}
	}
	return nil
}

func accountConfigGroupMappingEntries(value any) map[string]any {
	switch mapping := value.(type) {
	case map[string]any:
		return mapping
	case map[string]string:
		entries := make(map[string]any, len(mapping))
		for key, target := range mapping {
			entries[key] = target
		}
		return entries
	default:
		return nil
	}
}

func replaceAccountConfigGroupSettings(existing, owned, replacement map[string]any) map[string]any {
	result := make(map[string]any, len(existing)+len(replacement))
	for key, value := range existing {
		if _, managed := owned[key]; !managed {
			result[key] = value
		}
	}
	for key, value := range replacement {
		result[key] = value
	}
	return result
}

func replaceAccountConfigGroupCredentials(existing, previous, replacement map[string]any) map[string]any {
	owned := service.AccountConfigGroupCredentialSettings(existing)
	_, hadPlanOverride := previous["plan_type"]
	_, hasPlanOverride := replacement["plan_type"]
	if !hadPlanOverride && !hasPlanOverride {
		// Keep an independently detected tier when an unrelated setting is
		// saved in automatic mode. Clearing a previous override still removes it.
		delete(owned, "plan_type")
	}
	credentials := replaceAccountConfigGroupSettings(existing, owned, replacement)
	service.ApplyAccountConfigGroupEndpointOverride(credentials)
	return credentials
}

func accountConfigGroupMemberIDs(ctx context.Context, exec sqlExecutor, id int64) ([]int64, error) {
	ids := []int64{}
	if id == 0 {
		return ids, nil
	}
	rows, err := exec.QueryContext(ctx, `SELECT account_id FROM account_config_group_members WHERE account_config_group_id=$1 ORDER BY account_id`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *accountRepository) DeleteAccountConfigGroup(ctx context.Context, id int64) error {
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	client := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		client = tx.Client()
	}
	// Serialize release with membership writes before taking member row locks.
	rows, err := client.QueryContext(ctx, `SELECT id FROM account_config_groups WHERE id=$1 FOR UPDATE`, id)
	if err != nil {
		return err
	}
	found := rows.Next()
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if !found {
		return service.ErrAccountConfigGroupNotFound
	}
	memberIDs, err := accountConfigGroupMemberIDs(ctx, client, id)
	if err != nil {
		return err
	}
	if len(memberIDs) > 0 {
		rows, err = client.QueryContext(ctx, `SELECT id FROM accounts WHERE id=ANY($1) ORDER BY id FOR UPDATE`, pq.Array(memberIDs))
		if err != nil {
			return err
		}
		for rows.Next() {
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		// Release the reserved marker, not the materialized endpoint settings.
		// Standalone editors may now freely change individual protocol addresses.
		if _, err := client.ExecContext(ctx, `UPDATE accounts SET credentials=credentials-$2, updated_at=NOW() WHERE id=ANY($1) AND credentials ? $2`, pq.Array(memberIDs), service.AccountConfigGroupBaseURLOverrideKey); err != nil {
			return err
		}
	}
	// Deletion releases membership but intentionally retains the last applied configuration.
	result, err := client.ExecContext(ctx, `DELETE FROM account_config_groups WHERE id=$1`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return service.ErrAccountConfigGroupNotFound
	}
	for _, memberID := range memberIDs {
		if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &memberID, nil, nil); err != nil {
			return err
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
		for _, memberID := range memberIDs {
			r.syncSchedulerAccountSnapshot(ctx, memberID)
		}
	}
	return nil
}
