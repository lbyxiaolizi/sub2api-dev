package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// lockUngroupedAccountConfigWrites serializes direct writes with membership
// creation. The account row lock is the same lock SaveAccountConfigGroup takes.
// Runtime-only credential/extra observations deliberately skip this guard.
func lockUngroupedAccountConfigWrites(ctx context.Context, exec sqlExecutor, ids []int64) error {
	rows, err := exec.QueryContext(ctx, `/* account_config_group_write_guard */ SELECT id FROM accounts WHERE id = ANY($1) AND deleted_at IS NULL ORDER BY id FOR UPDATE`, pq.Array(ids))
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
	rows, err = exec.QueryContext(ctx, `SELECT account_id FROM account_config_group_members WHERE account_id = ANY($1) LIMIT 1`, pq.Array(ids))
	if err != nil {
		return err
	}
	managed := rows.Next()
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if managed {
		return service.ErrAccountConfigGroupManaged
	}
	return nil
}

func isAccountConfigGroupBulkSettingsWrite(updates service.AccountBulkUpdate) bool {
	return updates.ProxyID != nil || updates.Concurrency != nil || updates.Priority != nil || updates.RateMultiplier != nil || updates.LoadFactor != nil || updates.Status != nil || updates.Schedulable != nil || updates.ProbeEnabled != nil || len(service.AccountConfigGroupCredentialSettings(updates.Credentials)) > 0 || len(service.AccountConfigGroupExtraSettings(updates.Extra)) > 0
}
