package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type CreateAlertWithDeliveryParams struct {
	EnvID    uuid.UUID
	TenantID uuid.UUID
	Type     string
	Severity string
	Message  string
	Metadata json.RawMessage
}

func (store *SQLStore) CreateAlertWithDeliveryTx(ctx context.Context, arg CreateAlertWithDeliveryParams) (Alert, error) {

	var alert Alert

	err := store.execTx(ctx, func(q *Queries) error {

		var err error

		// 1. Create alert

		alert, err = q.CreateAlert(ctx, CreateAlertParams{
			EnvID:    arg.EnvID,
			Metadata: arg.Metadata,
			Type:     arg.Type,
			Severity: arg.Severity,
			Message:  arg.Message,
		})

		if err != nil {
			return fmt.Errorf("create alert: %w", err)
		}

		// 2. Get all active channels for this tenant
		channels, err := q.ListActiveNotificationChannels(ctx, arg.TenantID)

		if err != nil {
			return fmt.Errorf("create alert: %w", err)
		}

		for _, ch := range channels {

			_, err := q.CreateAlertDelivery(ctx, CreateAlertDeliveryParams{
				AlertID:   alert.ID,
				ChannelID: ch.ID,
			})
			if err != nil {
				return fmt.Errorf("create delivery for channel %s: %w", ch.ID, err)
			}
		}

		return nil
	})

	return alert, err

}
