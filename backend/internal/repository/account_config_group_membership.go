package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *accountRepository) GetAccountConfigGroupMemberships(ctx context.Context, ids []int64) (map[int64]service.AccountConfigGroupMembership, error) {
	out := make(map[int64]service.AccountConfigGroupMembership)
	ids = uniquePositiveInt64s(ids)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT m.account_id, g.id, g.name
		FROM account_config_group_members m
		JOIN account_config_groups g ON g.id = m.account_config_group_id
		WHERE m.account_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var accountID int64
		var membership service.AccountConfigGroupMembership
		if err := rows.Scan(&accountID, &membership.ID, &membership.Name); err != nil {
			return nil, err
		}
		out[accountID] = membership
	}
	return out, rows.Err()
}
