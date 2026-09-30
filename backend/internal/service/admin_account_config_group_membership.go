package service

import "context"

// AccountConfigGroupMembership is display-only metadata, loaded only for admin
// account responses rather than on the scheduler's hot read path.
type AccountConfigGroupMembership struct {
	ID   int64
	Name string
}

type AccountConfigGroupMembershipReader interface {
	GetAccountConfigGroupMemberships(context.Context, []int64) (map[int64]AccountConfigGroupMembership, error)
}

func (s *adminServiceImpl) enrichAccountConfigGroupMemberships(ctx context.Context, accounts []*Account) error {
	reader, ok := s.accountRepo.(AccountConfigGroupMembershipReader)
	if !ok || len(accounts) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			ids = append(ids, account.ID)
		}
	}
	memberships, err := reader.GetAccountConfigGroupMemberships(ctx, ids)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if account == nil {
			continue
		}
		account.AccountConfigGroupID = nil
		account.AccountConfigGroupName = ""
		if membership, found := memberships[account.ID]; found {
			id := membership.ID
			account.AccountConfigGroupID = &id
			account.AccountConfigGroupName = membership.Name
		}
	}
	return nil
}
