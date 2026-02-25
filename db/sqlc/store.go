package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Querier
	RegisterTenantTx(ctx context.Context, arg RegisterTenantParams) (RegisterTenantResult, error)
	DeleteEnvironmentTx(ctx context.Context, arg DeleteEnvironmentTxParams) error
	IngestErrorBatchTx(ctx context.Context, arg IngestErrorBatchParams) error
	CreateAlertWithDeliveryTx(ctx context.Context, arg CreateAlertWithDeliveryParams) (Alert, error)

	RunAnonymizationTx(ctx context.Context, arg RunAnonymizationTxParams) error
}

type SQLStore struct {
	*Queries
	connPoll *pgxpool.Pool
}

func NewStore(connPool *pgxpool.Pool) *SQLStore {

	return &SQLStore{
		Queries:  New(connPool),
		connPoll: connPool,
	}
}
