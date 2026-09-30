package repository

import (
	"context"
	"database/sql"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.AccountConfigGroupAccountRepository = (*accountRepository)(nil)

func stripAccountConfigGroupEndpointOverride(credentials map[string]any) map[string]any {
	if _, exists := credentials[service.AccountConfigGroupBaseURLOverrideKey]; !exists {
		return credentials
	}
	result := make(map[string]any, len(credentials)-1)
	for key, value := range credentials {
		if key != service.AccountConfigGroupBaseURLOverrideKey {
			result[key] = value
		}
	}
	return result
}

func (r *accountRepository) CreateAccountInConfigGroup(ctx context.Context, group *service.AccountConfigGroup, account *service.Account) error {
	if group == nil || group.ID <= 0 || account == nil || account.ID != 0 || !service.SupportsAccountConfigGroupKeyCreation(group) || account.Platform != group.Platform || account.Type != group.Type {
		return service.ErrAccountConfigGroupInvalid
	}
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}
	client := r.client
	if tx != nil {
		defer func() { _ = tx.Rollback() }()
		client = tx.Client()
	}
	// Do not publish an ID or mutate the caller's group if any later write fails.
	created := *account
	updatedGroup := *group
	updatedGroup.AccountIDs = append([]int64(nil), group.AccountIDs...)
	txRepo := newAccountRepositoryWithSQL(client, client, nil)
	if err := txRepo.CreateWithAccountGroups(ctx, &created, []service.AccountGroup{{GroupID: group.GroupID, Priority: 1}}); err != nil {
		return err
	}
	updatedGroup.AccountIDs = append(updatedGroup.AccountIDs, created.ID)
	// Save performs the live group/revision check, parent/type/member validation,
	// setting-dependent initialization and pool assignment within this same tx.
	if err := txRepo.SaveAccountConfigGroup(ctx, &updatedGroup); err != nil {
		return err
	}
	loaded, err := txRepo.GetByID(ctx, created.ID)
	if err != nil {
		return err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
		for _, id := range updatedGroup.AccountIDs {
			r.syncSchedulerAccountSnapshot(ctx, id)
		}
	}
	*account = *loaded
	*group = updatedGroup
	return nil
}

// Restore only the persisted override, never an arbitrary incoming identity
// marker. Callers already hold the account row lock. Partial key refreshes and
// stale full-account writes therefore cannot lose or inject endpoint overrides.
func restoreLockedAccountConfigGroupEndpoint(ctx context.Context, client *dbent.Client, id int64, group *service.AccountConfigGroup, credentials map[string]any) (map[string]any, error) {
	if !service.SupportsAccountConfigGroupKeyCreation(group) {
		return credentials, nil
	}
	rows, err := client.QueryContext(ctx, `SELECT credentials ->> $2 FROM accounts WHERE id=$1 AND deleted_at IS NULL`, id, service.AccountConfigGroupBaseURLOverrideKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAccountNotFound
	}
	var override sql.NullString
	if err := rows.Scan(&override); err != nil {
		return nil, err
	}
	copy := make(map[string]any, len(credentials)+1)
	for key, value := range credentials {
		if key != service.AccountConfigGroupBaseURLOverrideKey {
			copy[key] = value
		}
	}
	if override.Valid && override.String != "" {
		copy[service.AccountConfigGroupBaseURLOverrideKey] = override.String
	}
	return copy, rows.Err()
}
