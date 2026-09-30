package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func expectUngroupedAccountConfigWrite(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`(?s)/\* account_config_group_write_guard \*/ SELECT id FROM accounts`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT account_id FROM account_config_group_members WHERE account_id`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
}

func TestAccountConfigGroupBulkWriteGuardRejectsBeforeAnyWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)/\* account_config_group_write_guard \*/ SELECT id FROM accounts`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT account_id FROM account_config_group_members WHERE account_id`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(2)))
	mock.ExpectRollback()
	concurrency := 3
	count, err := newAccountRepositoryWithSQL(client, db, nil).BulkUpdate(context.Background(), []int64{1, 2}, service.AccountBulkUpdate{Concurrency: &concurrency})
	require.ErrorIs(t, err, service.ErrAccountConfigGroupManaged)
	require.Zero(t, count)
	require.NoError(t, mock.ExpectationsWereMet())
}
