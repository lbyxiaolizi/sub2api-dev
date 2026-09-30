package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func expectRuntimeAccountConfigGroup(mock sqlmock.Sqlmock, config *service.AccountConfigGroupConfig) {
	mock.ExpectQuery("SELECT proxy_id FROM accounts").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"proxy_id"}).AddRow(nil))
	rows := sqlmock.NewRows([]string{"platform", "type", "config"})
	if config != nil {
		raw, _ := json.Marshal(config)
		rows.AddRow(service.PlatformOpenAI, service.AccountTypeOAuth, raw)
	}
	mock.ExpectQuery("SELECT cg.platform, cg.type, cg.config").WithArgs(int64(1)).WillReturnRows(rows)
}

func TestAccountConfigGroupClearErrorRespectsCentralPause(t *testing.T) {
	cases := []struct {
		name   string
		config *service.AccountConfigGroupConfig
		status string
		paused bool
	}{
		{"ungrouped", nil, service.StatusActive, false},
		{"active group", &service.AccountConfigGroupConfig{Status: service.StatusActive, Schedulable: true}, service.StatusActive, false},
		{"inactive group", &service.AccountConfigGroupConfig{Status: "inactive", Schedulable: true}, "inactive", true},
		{"error group", &service.AccountConfigGroupConfig{Status: service.StatusError, Schedulable: true}, service.StatusError, true},
		{"unschedulable group", &service.AccountConfigGroupConfig{Status: service.StatusActive, Schedulable: false}, service.StatusActive, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			mock.ExpectBegin()
			expectRuntimeAccountConfigGroup(mock, tc.config)
			update := mock.ExpectExec(`UPDATE "accounts" SET`)
			if tc.paused {
				update.WithArgs(sqlmock.AnyArg(), tc.status, "", false, int64(1))
			} else {
				update.WithArgs(sqlmock.AnyArg(), tc.status, "", int64(1))
			}
			update.WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			require.NoError(t, newAccountRepositoryWithSQL(client, db, nil).ClearError(context.Background(), 1))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAccountConfigGroupSetSchedulableCannotReenableCentralPause(t *testing.T) {
	cases := []struct {
		name                string
		config              *service.AccountConfigGroupConfig
		requested, expected bool
	}{
		{"ungrouped recovery", nil, true, true},
		{"enabled group recovery", &service.AccountConfigGroupConfig{Status: service.StatusActive, Schedulable: true}, true, true},
		{"enabled group failure", &service.AccountConfigGroupConfig{Status: service.StatusActive, Schedulable: true}, false, false},
		{"inactive group recovery", &service.AccountConfigGroupConfig{Status: "inactive", Schedulable: true}, true, false},
		{"error group recovery", &service.AccountConfigGroupConfig{Status: service.StatusError, Schedulable: true}, true, false},
		{"unschedulable group recovery", &service.AccountConfigGroupConfig{Status: service.StatusActive, Schedulable: false}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			mock.ExpectBegin()
			expectRuntimeAccountConfigGroup(mock, tc.config)
			mock.ExpectExec(`UPDATE "accounts" SET`).WithArgs(sqlmock.AnyArg(), tc.expected, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			require.NoError(t, newAccountRepositoryWithSQL(client, db, nil).SetSchedulable(context.Background(), 1, tc.requested))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
