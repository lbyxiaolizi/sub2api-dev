package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type accountConfigGroupMembershipRepoStub struct {
	AccountRepository
	ids         []int64
	memberships map[int64]AccountConfigGroupMembership
	err         error
}

func (r *accountConfigGroupMembershipRepoStub) GetAccountConfigGroupMemberships(_ context.Context, ids []int64) (map[int64]AccountConfigGroupMembership, error) {
	r.ids = ids
	return r.memberships, r.err
}

func TestEnrichAccountConfigGroupMemberships(t *testing.T) {
	repo := &accountConfigGroupMembershipRepoStub{memberships: map[int64]AccountConfigGroupMembership{1: {ID: 8, Name: "shared"}}}
	svc := &adminServiceImpl{accountRepo: repo}
	staleID := int64(9)
	accounts := []*Account{{ID: 1}, nil, {ID: 2, AccountConfigGroupID: &staleID, AccountConfigGroupName: "stale"}}
	require.NoError(t, svc.enrichAccountConfigGroupMemberships(context.Background(), accounts))
	require.Equal(t, []int64{1, 2}, repo.ids)
	require.Equal(t, int64(8), *accounts[0].AccountConfigGroupID)
	require.Equal(t, "shared", accounts[0].AccountConfigGroupName)
	require.Nil(t, accounts[2].AccountConfigGroupID)
	require.Empty(t, accounts[2].AccountConfigGroupName)
}

func TestEnrichAccountConfigGroupMembershipsFailsClosed(t *testing.T) {
	repo := &accountConfigGroupMembershipRepoStub{err: errors.New("database unavailable")}
	svc := &adminServiceImpl{accountRepo: repo}
	require.EqualError(t, svc.enrichAccountConfigGroupMemberships(context.Background(), []*Account{{ID: 1}}), "database unavailable")
}
