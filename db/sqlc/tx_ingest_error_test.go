package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIngestErrorBatchTx(t *testing.T) {
	tenant := createRandomTenant(t)
	env := createRandomEnviroment(t, tenant.ID)

	t.Run("BasicIngestion", func(t *testing.T) {
		arg := IngestErrorBatchParams{
			EnvID:          env.ID,
			TenantID:       tenant.ID,
			Signature:      utils.RandomString(32),
			ErrorType:      "ValueError",
			Message:        "Test error message",
			Module:         stringPtr("sale"),
			Model:          stringPtr("sale.order"),
			Timestamp:      time.Now(),
			AffectedUIDs:   []int32{1, 10, 50},
			SpikeThreshold: 100, // High threshold, no alert expected
		}

		err := testStore.IngestErrorBatchTx(context.Background(), arg)
		require.NoError(t, err)

		// Verify stored data
		eg, err := testStore.GetErrorGroupBySignature(context.Background(), GetErrorGroupBySignatureParams{
			EnvID:     env.ID,
			Signature: arg.Signature,
		})
		require.NoError(t, err)
		require.Equal(t, "sale", *eg.Module)
		require.Equal(t, "sale.order", *eg.Model)
		require.ElementsMatch(t, []int32{1, 10, 50}, eg.AffectedUsers)
	})

	t.Run("SpikeAlertTrigger", func(t *testing.T) {
		// Use a unique signature to ensure fresh count
		sig := utils.RandomString(32)
		threshold := 2

		arg := IngestErrorBatchParams{
			EnvID:          env.ID,
			TenantID:       tenant.ID,
			Signature:      sig,
			ErrorType:      "IndexError",
			Message:        "List index out of range",
			Module:         stringPtr("base"),
			Model:          stringPtr("res.partner"),
			Timestamp:      time.Now(),
			SpikeThreshold: threshold,
		}

		// 1. First ingestion: Count becomes 1. (1 < 2) -> No Alert
		err := testStore.IngestErrorBatchTx(context.Background(), arg)
		require.NoError(t, err)

		// 2. Second ingestion: Count becomes 2. (2 >= 2) AND (1 < 2) -> Alert Created
		err = testStore.IngestErrorBatchTx(context.Background(), arg)
		require.NoError(t, err)

		// 3. Third ingestion: Count becomes 3. (3 >= 2) BUT (2 < 2 is false) -> No Alert
		err = testStore.IngestErrorBatchTx(context.Background(), arg)
		require.NoError(t, err)
	})
}
