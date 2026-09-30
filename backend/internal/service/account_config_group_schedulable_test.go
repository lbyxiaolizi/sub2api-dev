package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type configGroupSchedulableWriter struct {
	AccountRepository
	AccountConfigGroupRepository
	ids     []int64
	updates AccountBulkUpdate
	count   int64
	err     error
}

func (r *configGroupSchedulableWriter) GetAccountConfigGroupByAccount(context.Context, int64) (*AccountConfigGroup, error) {
	return nil, nil // The member can still be attached after this precheck.
}
func (r *configGroupSchedulableWriter) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	r.ids, r.updates = ids, updates
	return r.count, r.err
}
func (r *configGroupSchedulableWriter) GetByID(_ context.Context, id int64) (*Account, error) {
	return &Account{ID: id, Schedulable: *r.updates.Schedulable}, nil
}

func TestAccountConfigGroupManualSchedulingUsesAtomicWriteGuard(t *testing.T) {
	for _, tc := range []struct {
		name              string
		count             int64
		writeErr, wantErr error
	}{
		{"independent account", 1, nil, nil},
		{"attached after precheck", 0, ErrAccountConfigGroupManaged, ErrAccountConfigGroupManaged},
		{"deleted after precheck", 0, nil, ErrAccountNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &configGroupSchedulableWriter{count: tc.count, err: tc.writeErr}
			svc := &adminServiceImpl{accountRepo: repo}
			account, err := svc.SetAccountSchedulable(context.Background(), 1, false)
			require.Equal(t, []int64{1}, repo.ids)
			require.NotNil(t, repo.updates.Schedulable)
			require.False(t, *repo.updates.Schedulable)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, account)
			} else {
				require.NoError(t, err)
				require.False(t, account.Schedulable)
			}
		})
	}
}
