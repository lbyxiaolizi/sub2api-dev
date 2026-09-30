package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// The account row lock serializes this read with membership/configuration writes.
// In particular an OAuth refresh using a stale Account snapshot must not revert
// settings that were just changed through its configuration group.
func lockedAccountConfigGroup(ctx context.Context, client *dbent.Client, id int64) (*service.AccountConfigGroup, *int64, error) {
	// Finish the lock statement before reading the group. Under READ COMMITTED,
	// a join in the lock statement could retain a pre-wait snapshot of cg.config.
	rows, err := client.QueryContext(ctx, `SELECT proxy_id FROM accounts
		WHERE id = $1 AND deleted_at IS NULL FOR NO KEY UPDATE`, id)
	if err != nil {
		return nil, nil, err
	}
	if !rows.Next() {
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, service.ErrAccountNotFound
	}
	var proxyID sql.NullInt64
	err = rows.Scan(&proxyID)
	if err == nil {
		err = rows.Err()
	}
	_ = rows.Close()
	if err != nil {
		return nil, nil, err
	}

	rows, err = client.QueryContext(ctx, `SELECT cg.platform, cg.type, cg.config
		FROM account_config_group_members m
		JOIN account_config_groups cg ON cg.id = m.account_config_group_id
		WHERE m.account_id = $1`, id)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, nil, rows.Err()
	}
	group := &service.AccountConfigGroup{}
	var raw []byte
	if err := rows.Scan(&group.Platform, &group.Type, &raw); err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(raw, &group.Config); err != nil {
		return nil, nil, err
	}
	var proxy *int64
	if proxyID.Valid {
		proxy = &proxyID.Int64
	}
	return group, proxy, rows.Err()
}

func preserveAccountConfigGroup(account *service.Account, group *service.AccountConfigGroup, assignedProxyID *int64) {
	config := group.Config
	account.Platform, account.Type = group.Platform, group.Type
	account.Notes, account.PoolID = config.Notes, config.PoolID
	account.ProxyID = config.ProxyID
	if config.PoolID != nil {
		// The pool maintains per-account healthy-proxy assignments at runtime.
		account.ProxyID = assignedProxyID
	}
	account.Concurrency, account.Priority = config.Concurrency, config.Priority
	account.RateMultiplier, account.LoadFactor = &config.RateMultiplier, config.LoadFactor
	account.ExpiresAt = nil
	if config.ExpiresAt != nil {
		expiry := time.Unix(*config.ExpiresAt, 0)
		account.ExpiresAt = &expiry
	}
	account.AutoPauseOnExpired = config.AutoPauseOnExpired
	account.DisableAutoTempUnschedulable = config.DisableAutoTempUnschedulable
	// Runtime failures can still disable individual members, but a background
	// refresh must never re-enable a group explicitly paused by an administrator.
	if config.Status != service.StatusActive {
		account.Status = config.Status
	}
	if !config.Schedulable {
		account.Schedulable = false
	}
	account.Credentials = service.MergeAccountConfigGroupSettings(account.Credentials, config.Credentials, true)
	account.Extra = service.MergeAccountConfigGroupSettings(account.Extra, config.Extra, false)
}
