package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// createTestChannel creates an active notification channel for a tenant.
func createTestChannel(t *testing.T, tenantID uuid.UUID, chType string) NotificationChannel {
	t.Helper()

	ch, err := testStore.CreateNotificationChannel(context.Background(), CreateNotificationChannelParams{
		TenantID: tenantID,
		Name:     "Test Channel " + chType,
		Type:     chType,
		Config:   json.RawMessage(`{"url": "https://hooks.example.com/test"}`),
		IsActive: true,
	})
	require.NoError(t, err)
	require.NotZero(t, ch.ID)
	return ch
}

// createInactiveChannel creates a notification channel with is_active = false.
func createInactiveChannel(t *testing.T, tenantID uuid.UUID) NotificationChannel {
	t.Helper()

	ch, err := testStore.CreateNotificationChannel(context.Background(), CreateNotificationChannelParams{
		TenantID: tenantID,
		Name:     "Inactive Channel",
		Type:     "Slack",
		Config:   json.RawMessage(`{"url": "https://hooks.example.com/test"}`),
		IsActive: false,
	})
	require.NoError(t, err)

	return ch

}

// buildAlertArg builds a valid CreateAlertWithDeliveryParams.
func buildAlertArg(alertType, severity string, envID, tenantID uuid.UUID) CreateAlertWithDeliveryParams {
	return CreateAlertWithDeliveryParams{
		EnvID:    envID,
		TenantID: tenantID,
		Type:     alertType,
		Severity: severity,
		Message:  "Test alert: " + alertType,
		Metadata: json.RawMessage(`{"model": "product.product", "threshold": 15}`),
	}
}

func TestCreateAlertWithDeliveryTx_NoChannels_CreatesAlertOnly(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Test Env",
		OdooUrl:      "https://odoo.example.com",
		DbName:       "odoo_testt",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	arg := buildAlertArg("critical", "error_spike", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	// Alert must be populated (catches the shadowed variable bug)
	require.NotZero(t, alert.ID)
	require.Equal(t, arg.Type, alert.Type)
	require.Equal(t, arg.Severity, alert.Severity)
	require.Equal(t, arg.Message, alert.Message)
	require.Equal(t, env.ID, alert.EnvID)
	require.False(t, alert.Acknowledged)
	require.NotZero(t, alert.CreatedAt)

	// No deliveries should exist (no channels)
	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)

	require.NoError(t, err)
	require.Empty(t, deliveries, "no channels means no deliveries should exist")
}

func TestCreateAlertWithDeliveryTx_OneChannel_CreateAlertAndDelivery(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Prod Env",
		OdooUrl:      "https://prod.example.com",
		DbName:       "odoo_prod",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	// Create one active channel

	ch := createTestChannel(t, reg.Tenant.ID, "slack")
	arg := buildAlertArg("budget_exceeded", "warning", env.ID, reg.Tenant.ID)

	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)
	require.NotZero(t, alert.ID)

	// Exactly 1 delivery must exist
	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)

	d := deliveries[0]
	require.Equal(t, alert.ID, d.AlertID)
	require.Equal(t, ch.ID, d.ChannelID)
	require.Equal(t, "pending", d.Status)
	require.Equal(t, int32(1), d.Attempt)
}

func TestCreateAlertWithDeliveryTx_MultipleChannels_CreatesDeliveryForEach(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Multi Channel Env",
		OdooUrl:      "https://multi.example.com",
		DbName:       "odoo_multi",
		EnvType:      "staging",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Create 3 active channels of different type
	ch1 := createTestChannel(t, reg.Tenant.ID, "slack")
	ch2 := createTestChannel(t, reg.Tenant.ID, "email")
	ch3 := createTestChannel(t, reg.Tenant.ID, "webhook")

	arg := buildAlertArg("n1_query", "warning", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)
	require.NotZero(t, alert.ID)

	// Exactly 3 deliveries — one per channel
	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 3)

	// Collect channel IDs from deliveries

	deliveredChannelIDs := make(map[string]bool)

	for _, d := range deliveries {
		deliveredChannelIDs[d.ChannelID.String()] = true
		require.Equal(t, alert.ID, d.AlertID)
		require.Equal(t, "pending", d.Status)
	}

	// All 3 channels must have a delivery
	require.True(t, deliveredChannelIDs[ch1.ID.String()])
	require.True(t, deliveredChannelIDs[ch2.ID.String()])
	require.True(t, deliveredChannelIDs[ch3.ID.String()])
}

func TestCreateAlertWithDeliveryTx_InactiveChannels_NotDelivered(t *testing.T) {

	reg := createRegisteredTenant(t)
	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Inactive Test Env",
		OdooUrl:      "https://inactive.example.com",
		DbName:       "odoo_inactive",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// 1 active + 1 inactive channel
	activeChannel := createTestChannel(t, reg.Tenant.ID, "slack")

	_ = createInactiveChannel(t, reg.Tenant.ID)

	arg := buildAlertArg("error_spike", "critical", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)

	// Only 1 delivery — the inactive channel must be skipped
	require.Len(t, deliveries, 1)
	require.Equal(t, activeChannel.ID, deliveries[0].ChannelID)

}
func TestCreateAlertWithDeliveryTx_AllChannelsInactive_NoDeliveries(t *testing.T) {

	// reg := createRegisteredTenant(t)
	reg := createRegisteredTenant(t)
	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "All Inactive Env",
		OdooUrl:      "https://allinactive.example.com",
		DbName:       "odoo_allinactive",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})

	require.NoError(t, err)
	_ = createInactiveChannel(t, reg.Tenant.ID)
	_ = createInactiveChannel(t, reg.Tenant.ID)

	arg := buildAlertArg("agent_offline", "critical", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)

	require.NoError(t, err)
	require.NotZero(t, alert.ID)

	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Empty(t, deliveries, "inactive channels must not receive deliveries")

}
func TestCreateAlertWithDeliveryTx_AllSeverityLevels(t *testing.T) {

	reg := createRegisteredTenant(t)
	// reg := createRegisteredTenant(t)
	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Severity Test Env",
		OdooUrl:      "https://severity.example.comm",
		DbName:       "odoo_severity",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})

	require.NoError(t, err)

	severities := []string{"critical", "warning", "info"}

	for _, severity := range severities {
		t.Run(severity, func(t *testing.T) {
			arg := buildAlertArg("budget_exceeded", severity, env.ID, reg.Tenant.ID)

			alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
			require.NoError(t, err)
			require.Equal(t, severity, alert.Severity)
			require.NotZero(t, alert.ID)
		})
	}

}
func TestCreateAlertWithDeliveryTx_AllAlertTypes(t *testing.T) {

	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Types Test Env",
		OdooUrl:      "https://types.example.com",
		DbName:       "odoo_types",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	alertTypes := []string{"n1_query", "budget_exceeded", "error_spike", "agent_offline"}

	for _, alertType := range alertTypes {
		t.Run(alertType, func(t *testing.T) {
			arg := buildAlertArg(alertType, "warning", env.ID, reg.Tenant.ID)

			alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
			require.NoError(t, err)
			require.Equal(t, alertType, alert.Type)
			require.NotZero(t, alert.ID)
		})
	}

}
func TestCreateAlertWithDeliveryTx_DefaultsAcknowledgedFalse(t *testing.T) {

	reg := createRegisteredTenant(t)
	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Ack Test Env",
		OdooUrl:      "https://ack.example.com",
		DbName:       "odoo_ack",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	arg := buildAlertArg("error_spike", "critical", env.ID, reg.Tenant.ID)

	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	require.False(t, alert.Acknowledged)
	require.Nil(t, alert.AcknowledgedBy)
	require.Nil(t, alert.AcknowledgedAt)
}
func TestCreateAlertWithDeliveryTx_InvalidEnvID_RollsBack(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Use a random EnvID that doesn't exist → CreateAlert must fail (FK violation)
	arg := CreateAlertWithDeliveryParams{
		EnvID:    utils.RandomUUID(), // does not exist
		TenantID: reg.Tenant.ID,
		Type:     "error_spike",
		Severity: "critical",
		Message:  "This should fail",
		Metadata: json.RawMessage(`{}`),
	}

	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.Error(t, err, "invalid env_id must cause transaction to fail")

	// Returned alert must be zero-value — nothing committed
	require.Zero(t, alert.ID)
}
func TestCreateAlertWithDeliveryTx_OnlyDeliversToOwnTenantChannels(t *testing.T) {
	// Two tenants, each with their own channel
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	env1, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg1.Tenant.ID,
		Name:         "Tenant1 Env",
		OdooUrl:      "https://t1.example.com",
		DbName:       "odoo_t1",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	ch1 := createTestChannel(t, reg1.Tenant.ID, "slack") // tenant 1 channel
	_ = createTestChannel(t, reg2.Tenant.ID, "email")    // tenant 2 channel — must NOT receive delivery

	arg := buildAlertArg("error_spike", "critical", env1.ID, reg1.Tenant.ID)

	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)

	// Only tenant 1's channel gets a delivery
	require.Len(t, deliveries, 1)
	require.Equal(t, ch1.ID, deliveries[0].ChannelID)
}
func TestCreateAlertWithDeliveryTx_AlertPersistsToDatabase(t *testing.T) {
	reg := createRegisteredTenant(t)
	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Persist Test Env",
		OdooUrl:      "https://persist.example.com",
		DbName:       "odoo_persist",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	_ = createTestChannel(t, reg.Tenant.ID, "webhook")

	arg := buildAlertArg("budget_exceeded", "warning", env.ID, reg.Tenant.ID)

	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	// Fetch alert from DB and compare
	fetchedAlert, err := testStore.GetAlertByID(context.Background(), GetAlertByIDParams{
		ID:    alert.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, alert.ID, fetchedAlert.ID)
	require.Equal(t, alert.Type, fetchedAlert.Type)
	require.Equal(t, alert.Severity, fetchedAlert.Severity)
	require.Equal(t, alert.Message, fetchedAlert.Message)

	// Fetch deliveries and verify they're also persisted
	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, "pending", deliveries[0].Status)
}
func TestCreateAlertWithDeliveryTx_MetadataRoundTrip(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Metadata Test Env",
		OdooUrl:      "https://metadata.example.com",
		DbName:       "odoo_metadata",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	inputMeta := json.RawMessage(`{"model":"product.product","threshold":15,"tags":["perf","critical"]}`)

	arg := CreateAlertWithDeliveryParams{
		EnvID:    env.ID,
		TenantID: reg.Tenant.ID,
		Type:     "n1_query",
		Severity: "warning",
		Message:  "Metadata round-trip test",
		Metadata: inputMeta,
	}

	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	// Compare metadata on returned alert
	var expected, actual map[string]any
	require.NoError(t, json.Unmarshal(inputMeta, &expected))
	require.NoError(t, json.Unmarshal(alert.Metadata, &actual))
	require.Equal(t, expected, actual)

	// Also verify from a fresh DB read
	fetched, err := testStore.GetAlertByID(context.Background(), GetAlertByIDParams{
		ID:    alert.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)

	var fetchedMeta map[string]any
	require.NoError(t, json.Unmarshal(fetched.Metadata, &fetchedMeta))
	require.Equal(t, expected, fetchedMeta)
}
func TestCreateAlertWithDeliveryTx_EmptyMetadata(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Empty Meta Env",
		OdooUrl:      "https://emptymeta.example.com",
		DbName:       "odoo_emptymeta",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	t.Run("empty_json_object", func(t *testing.T) {
		arg := CreateAlertWithDeliveryParams{
			EnvID:    env.ID,
			TenantID: reg.Tenant.ID,
			Type:     "error_spike",
			Severity: "info",
			Message:  "empty object metadata",
			Metadata: json.RawMessage(`{}`),
		}
		alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
		require.NoError(t, err)
		require.NotZero(t, alert.ID)

		var meta map[string]any
		require.NoError(t, json.Unmarshal(alert.Metadata, &meta))
		require.Empty(t, meta)
	})

	t.Run("null_json", func(t *testing.T) {
		arg := CreateAlertWithDeliveryParams{
			EnvID:    env.ID,
			TenantID: reg.Tenant.ID,
			Type:     "error_spike",
			Severity: "info",
			Message:  "null metadata",
			Metadata: json.RawMessage(`null`),
		}
		alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
		// Depending on DB column constraints, this either succeeds with null or fails.
		// We just verify consistency — no panic.
		if err == nil {
			require.NotZero(t, alert.ID)
		}
	})
}
func TestCreateAlertWithDeliveryTx_DuplicateChannelTypes_BothDelivered(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Dup Channel Env",
		OdooUrl:      "https://dupchannel.example.com",
		DbName:       "odoo_dupchannel",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Two active channels of the same type
	ch1 := createTestChannel(t, reg.Tenant.ID, "slack")
	ch2 := createTestChannel(t, reg.Tenant.ID, "slack")

	arg := buildAlertArg("error_spike", "critical", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 2, "both slack channels must receive a delivery")

	ids := map[string]bool{}
	for _, d := range deliveries {
		ids[d.ChannelID.String()] = true
	}
	require.True(t, ids[ch1.ID.String()])
	require.True(t, ids[ch2.ID.String()])
}
func TestCreateAlertWithDeliveryTx_InvalidTenantID_NoDeliveries(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Bad Tenant Env",
		OdooUrl:      "https://badtenant.example.com",
		DbName:       "odoo_badtenant",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Create channel for the real tenant
	_ = createTestChannel(t, reg.Tenant.ID, "slack")

	// Pass a random tenant_id — channels lookup will find nothing
	arg := CreateAlertWithDeliveryParams{
		EnvID:    env.ID,
		TenantID: utils.RandomUUID(), // non-existent tenant
		Type:     "error_spike",
		Severity: "critical",
		Message:  "wrong tenant",
		Metadata: json.RawMessage(`{}`),
	}

	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err, "alert creation should still succeed (env_id is valid)")
	require.NotZero(t, alert.ID)

	// No deliveries because the random tenant has no channels
	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Empty(t, deliveries)
}
func TestCreateAlertWithDeliveryTx_ManyChannels(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Many Channels Env",
		OdooUrl:      "https://manychannels.example.com",
		DbName:       "odoo_manychannels",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	const channelCount = 15
	for range channelCount {
		createTestChannel(t, reg.Tenant.ID, "webhook")
	}

	arg := buildAlertArg("budget_exceeded", "warning", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)
	require.NotZero(t, alert.ID)

	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, channelCount, "every active channel must receive a delivery")

	// All deliveries must be pending with attempt=1
	for _, d := range deliveries {
		require.Equal(t, "pending", d.Status)
		require.Equal(t, int32(1), d.Attempt)
	}
}
func TestCreateAlertWithDeliveryTx_ConcurrentCreation(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Concurrent Env",
		OdooUrl:      "https://concurrent.example.com",
		DbName:       "odoo_concurrent",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	ch := createTestChannel(t, reg.Tenant.ID, "slack")

	const goroutines = 10

	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	alerts := make([]Alert, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			arg := buildAlertArg("error_spike", "critical", env.ID, reg.Tenant.ID)
			alerts[idx], errs[idx] = testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
		}(i)
	}

	wg.Wait()

	// Every goroutine must succeed
	for i := range goroutines {
		require.NoError(t, errs[i], "goroutine %d failed", i)
		require.NotZero(t, alerts[i].ID, "goroutine %d returned zero alert", i)
	}

	// Each alert must have exactly 1 delivery to the channel
	for i := 0; i < goroutines; i++ {
		deliveries, err := testStore.ListAlertDeliveries(context.Background(), alerts[i].ID)
		require.NoError(t, err)
		require.Len(t, deliveries, 1)
		require.Equal(t, ch.ID, deliveries[0].ChannelID)
	}
}
func TestCreateAlertWithDeliveryTx_DeliveryDefaults(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Delivery Defaults Env",
		OdooUrl:      "https://deliverydefaults.example.com",
		DbName:       "odoo_deliverydefaults",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	_ = createTestChannel(t, reg.Tenant.ID, "email")

	arg := buildAlertArg("agent_offline", "critical", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)

	d := deliveries[0]
	require.Equal(t, "pending", d.Status)
	require.Equal(t, int32(1), d.Attempt)
	require.Nil(t, d.Error, "new delivery must not have an error")
	require.Nil(t, d.SentAt, "new delivery must not have sent_at")
	require.NotZero(t, d.CreatedAt)
	require.NotZero(t, d.ID)
}
func TestCreateAlertWithDeliveryTx_MultipleAlertsSameEnv_DeliveriesIsolated(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Multi Alert Env",
		OdooUrl:      "https://multialert.example.com",
		DbName:       "odoo_multialert",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	_ = createTestChannel(t, reg.Tenant.ID, "slack")
	_ = createTestChannel(t, reg.Tenant.ID, "email")

	// Create two separate alerts on the same env
	arg1 := buildAlertArg("error_spike", "critical", env.ID, reg.Tenant.ID)
	alert1, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg1)
	require.NoError(t, err)

	arg2 := buildAlertArg("budget_exceeded", "warning", env.ID, reg.Tenant.ID)
	alert2, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg2)
	require.NoError(t, err)

	// Each alert must have its own set of 2 deliveries
	del1, err := testStore.ListAlertDeliveries(context.Background(), alert1.ID)
	require.NoError(t, err)
	require.Len(t, del1, 2)

	del2, err := testStore.ListAlertDeliveries(context.Background(), alert2.ID)
	require.NoError(t, err)
	require.Len(t, del2, 2)

	// Deliveries must reference their own alert, not the other
	for _, d := range del1 {
		require.Equal(t, alert1.ID, d.AlertID)
	}
	for _, d := range del2 {
		require.Equal(t, alert2.ID, d.AlertID)
	}
}
func TestCreateAlertWithDeliveryTx_TimestampsPopulated(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Timestamp Env",
		OdooUrl:      "https://timestamp.example.com",
		DbName:       "odoo_timestamp",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	_ = createTestChannel(t, reg.Tenant.ID, "webhook")

	arg := buildAlertArg("n1_query", "info", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	require.NotZero(t, alert.CreatedAt, "alert created_at must be set")

	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.NotZero(t, deliveries[0].CreatedAt, "delivery created_at must be set")
}
func TestCreateAlertWithDeliveryTx_PendingDeliveriesListedCorrectly(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Pending List Env",
		OdooUrl:      "https://pendinglist.example.com",
		DbName:       "odoo_pendinglist",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	_ = createTestChannel(t, reg.Tenant.ID, "slack")

	arg := buildAlertArg("error_spike", "critical", env.ID, reg.Tenant.ID)
	alert, err := testStore.CreateAlertWithDeliveryTx(context.Background(), arg)
	require.NoError(t, err)

	deliveries, err := testStore.ListAlertDeliveries(context.Background(), alert.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, "pending", deliveries[0].Status)
}
