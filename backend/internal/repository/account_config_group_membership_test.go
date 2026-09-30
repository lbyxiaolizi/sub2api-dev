package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetAccountConfigGroupMemberships(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountRepository{sql: db}
	mock.ExpectQuery("SELECT m.account_id, g.id, g.name").WithArgs("{1,2}").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "id", "name"}).AddRow(1, 4, "shared"))
	result, err := repo.GetAccountConfigGroupMemberships(context.Background(), []int64{1, 2, 1, -1})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, int64(4), result[1].ID)
	require.Equal(t, "shared", result[1].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAccountConfigGroupMembershipsEmpty(t *testing.T) {
	repo := &accountRepository{}
	result, err := repo.GetAccountConfigGroupMemberships(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestGetAccountConfigGroupMembershipsPropagatesQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountRepository{sql: db}
	mock.ExpectQuery("SELECT m.account_id, g.id, g.name").WillReturnError(errors.New("database unavailable"))
	_, err = repo.GetAccountConfigGroupMemberships(context.Background(), []int64{1})
	require.EqualError(t, err, "database unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}
