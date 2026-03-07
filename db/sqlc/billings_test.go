package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string        { return &s }
func timePtr(t time.Time) *time.Time { return &t }

func cleanupSubscription(t *testing.T, tenantID uuid.UUID) {
	t.Helper()
	_ = testStore.DeleteSubscription(context.Background(), tenantID)
}
func createTestSubscription(t *testing.T, tenantID uuid.UUID) Subscription {
	t.Helper()
	// Delete any existing subscription to avoid unique constraint violations
	cleanupSubscription(t, tenantID)

	custID := "cus_" + utils.RandomString(14)
	subID := "sub_" + utils.RandomString(14)
	priceID := "price_" + utils.RandomString(14)
	now := time.Now().UTC().Truncate(time.Second)

	sub, err := testStore.CreateSubscription(context.Background(), CreateSubscriptionParams{
		TenantID:             tenantID,
		StripeCustomerID:     strPtr(custID),
		StripeSubscriptionID: strPtr(subID),
		StripePriceID:        strPtr(priceID),
		Plan:                 "pro",
		Status:               "active",
		CurrentPeriodStart:   timePtr(now),
		CurrentPeriodEnd:     timePtr(now.Add(30 * 24 * time.Hour)),
	})
	require.NoError(t, err)
	require.NotZero(t, sub.ID)
	return sub
}
func createTestBillingEvent(t *testing.T, eventType string) BillingEvent {
	t.Helper()
	evt, err := testStore.CreateBillingEvent(context.Background(), CreateBillingEventParams{
		StripeEventID: "evt_" + utils.RandomString(16),
		EventType:     eventType,
		Payload:       json.RawMessage(`{"type":"` + eventType + `"}`),
	})
	require.NoError(t, err)
	require.NotZero(t, evt.ID)
	return evt
}
func TestCreateSubscription_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Clean up any pre-existing subscription for this tenant
	// (e.g. if createRegisteredTenant seeds one automatically)
	_ = testStore.DeleteSubscription(context.Background(), reg.Tenant.ID)

	now := time.Now().UTC().Truncate(time.Second)
	trialEnd := now.Add(14 * 24 * time.Hour)
	custID := "cus_" + utils.RandomString(14)
	subID := "sub_" + utils.RandomString(14)
	priceID := "price_" + utils.RandomString(14)

	sub, err := testStore.CreateSubscription(context.Background(), CreateSubscriptionParams{
		TenantID:             reg.Tenant.ID,
		StripeCustomerID:     strPtr(custID),
		StripeSubscriptionID: strPtr(subID),
		StripePriceID:        strPtr(priceID),
		Plan:                 "enterprise",
		Status:               "trialing",
		CurrentPeriodStart:   timePtr(now),
		CurrentPeriodEnd:     timePtr(now.Add(30 * 24 * time.Hour)),
		TrialEnd:             timePtr(trialEnd),
	})
	require.NoError(t, err)

	require.NotZero(t, sub.ID)
	require.Equal(t, reg.Tenant.ID, sub.TenantID)
	require.Equal(t, custID, *sub.StripeCustomerID)
	require.Equal(t, subID, *sub.StripeSubscriptionID)
	require.Equal(t, priceID, *sub.StripePriceID)
	require.Equal(t, "enterprise", sub.Plan)
	require.Equal(t, "trialing", sub.Status)
	require.NotNil(t, sub.CurrentPeriodStart)
	require.NotNil(t, sub.CurrentPeriodEnd)
	require.NotNil(t, sub.TrialEnd)
	require.False(t, sub.CancelAtPeriodEnd)
	require.NotZero(t, sub.CreatedAt)
	require.NotZero(t, sub.UpdatedAt)
}
func TestCreateSubscription_MinimalFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	cleanupSubscription(t, reg.Tenant.ID)
	sub, err := testStore.CreateSubscription(context.Background(), CreateSubscriptionParams{
		TenantID: reg.Tenant.ID,
		Plan:     "free",
		Status:   "active",
	})
	require.NoError(t, err)

	require.NotZero(t, sub.ID)
	require.Nil(t, sub.StripeCustomerID)
	require.Nil(t, sub.StripeSubscriptionID)
	require.Nil(t, sub.StripePriceID)
	require.Nil(t, sub.CurrentPeriodStart)
	require.Nil(t, sub.CurrentPeriodEnd)
	require.Nil(t, sub.TrialEnd)
}
func TestGetSubscriptionByTenant_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	created := createTestSubscription(t, reg.Tenant.ID)

	fetched, err := testStore.GetSubscriptionByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.Plan, fetched.Plan)
	require.Equal(t, created.Status, fetched.Status)
}
func TestGetSubscriptionByTenant_NotFound(t *testing.T) {
	reg := createRegisteredTenant(t)
	cleanupSubscription(t, reg.Tenant.ID)

	_, err := testStore.GetSubscriptionByTenant(context.Background(), reg.Tenant.ID)
	require.Error(t, err)
}
func TestGetSubscriptionByStripeCustomer_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	created := createTestSubscription(t, reg.Tenant.ID)

	fetched, err := testStore.GetSubscriptionByStripeCustomer(context.Background(), created.StripeCustomerID)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, reg.Tenant.ID, fetched.TenantID)
}
func TestGetSubscriptionByStripeCustomer_NotFound(t *testing.T) {
	nonexistent := "cus_nonexistent"
	_, err := testStore.GetSubscriptionByStripeCustomer(context.Background(), &nonexistent)
	require.Error(t, err)
}
func TestGetSubscriptionByStripeID_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	created := createTestSubscription(t, reg.Tenant.ID)

	fetched, err := testStore.GetSubscriptionByStripeID(context.Background(), created.StripeSubscriptionID)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
}
func TestGetSubscriptionByStripeID_NotFound(t *testing.T) {
	nonexistent := "sub_nonexistent"
	_, err := testStore.GetSubscriptionByStripeID(context.Background(), &nonexistent)
	require.Error(t, err)
}
func TestUpdateSubscriptionStatus_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	_ = createTestSubscription(t, reg.Tenant.ID)
	newStart := time.Now().UTC().Truncate(time.Second)
	newEnd := newStart.Add(30 * 24 * time.Hour)
	newPriceID := "price_upgraded"

	updated, err := testStore.UpdateSubscriptionStatus(context.Background(), UpdateSubscriptionStatusParams{
		TenantID:           reg.Tenant.ID,
		Status:             "past_due",
		Plan:               "enterprise",
		StripePriceID:      strPtr(newPriceID),
		CurrentPeriodStart: timePtr(newStart),
		CurrentPeriodEnd:   timePtr(newEnd),
		CancelAtPeriodEnd:  true,
	})
	require.NoError(t, err)

	require.Equal(t, "past_due", updated.Status)
	require.Equal(t, "enterprise", updated.Plan)
	require.Equal(t, newPriceID, *updated.StripePriceID)
	require.True(t, updated.CancelAtPeriodEnd)
	require.NotNil(t, updated.CurrentPeriodStart)
	require.NotNil(t, updated.CurrentPeriodEnd)
}
func TestUpdateSubscriptionStatus_UpdatedAtAdvances(t *testing.T) {
	reg := createRegisteredTenant(t)
	original := createTestSubscription(t, reg.Tenant.ID)

	updated, err := testStore.UpdateSubscriptionStatus(context.Background(), UpdateSubscriptionStatusParams{
		TenantID:          reg.Tenant.ID,
		Status:            "canceled",
		Plan:              original.Plan,
		CancelAtPeriodEnd: false,
	})
	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(original.UpdatedAt) ||
		updated.UpdatedAt.Equal(original.UpdatedAt))
}
func TestUpdateSubscriptionStatus_NonexistentTenant(t *testing.T) {
	reg := createRegisteredTenant(t)
	cleanupSubscription(t, reg.Tenant.ID)
	_, err := testStore.UpdateSubscriptionStatus(context.Background(), UpdateSubscriptionStatusParams{
		TenantID: reg.Tenant.ID, // no subscription for this tenant
		Status:   "active",
		Plan:     "pro",
	})
	require.Error(t, err)
}
func TestUpdateSubscriptionStatus_PreservesStripeIDs(t *testing.T) {
	reg := createRegisteredTenant(t)
	original := createTestSubscription(t, reg.Tenant.ID)

	updated, err := testStore.UpdateSubscriptionStatus(context.Background(), UpdateSubscriptionStatusParams{
		TenantID:          reg.Tenant.ID,
		Status:            "active",
		Plan:              "enterprise",
		CancelAtPeriodEnd: false,
	})
	require.NoError(t, err)

	// stripe_customer_id and stripe_subscription_id must be untouched
	require.Equal(t, original.StripeCustomerID, updated.StripeCustomerID)
	require.Equal(t, original.StripeSubscriptionID, updated.StripeSubscriptionID)
}
func TestUpdateSubscriptionStripeIDs_Updates(t *testing.T) {
	reg := createRegisteredTenant(t)
	_ = createTestSubscription(t, reg.Tenant.ID)

	newCustID := "cus_" + utils.RandomString(14)
	newSubID := "sub_" + utils.RandomString(14)
	updated, err := testStore.UpdateSubscriptionStripeIDs(context.Background(), UpdateSubscriptionStripeIDsParams{
		TenantID:             reg.Tenant.ID,
		StripeCustomerID:     strPtr(newCustID),
		StripeSubscriptionID: strPtr(newSubID),
	})
	require.NoError(t, err)
	require.Equal(t, newCustID, *updated.StripeCustomerID)
	require.Equal(t, newSubID, *updated.StripeSubscriptionID)
}
func TestUpdateSubscriptionStripeIDs_PreservesOtherFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	original := createTestSubscription(t, reg.Tenant.ID)

	updated, err := testStore.UpdateSubscriptionStripeIDs(context.Background(), UpdateSubscriptionStripeIDsParams{
		TenantID:             reg.Tenant.ID,
		StripeCustomerID:     strPtr("cus_" + utils.RandomString(14)),
		StripeSubscriptionID: strPtr("sub_" + utils.RandomString(14)),
	})
	require.NoError(t, err)

	require.Equal(t, original.Plan, updated.Plan)
	require.Equal(t, original.Status, updated.Status)
	require.Equal(t, original.StripePriceID, updated.StripePriceID)
	require.Equal(t, original.CancelAtPeriodEnd, updated.CancelAtPeriodEnd)
}
func TestUpdateSubscriptionStripeIDs_UpdatedAtAdvances(t *testing.T) {
	reg := createRegisteredTenant(t)
	original := createTestSubscription(t, reg.Tenant.ID)

	updated, err := testStore.UpdateSubscriptionStripeIDs(context.Background(), UpdateSubscriptionStripeIDsParams{
		TenantID:             reg.Tenant.ID,
		StripeCustomerID:     strPtr("cus_" + utils.RandomString(14)),
		StripeSubscriptionID: strPtr("sub_" + utils.RandomString(14)),
	})
	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(original.UpdatedAt) ||
		updated.UpdatedAt.Equal(original.UpdatedAt))
}
func TestDeleteSubscription_Deletes(t *testing.T) {
	reg := createRegisteredTenant(t)
	_ = createTestSubscription(t, reg.Tenant.ID)

	err := testStore.DeleteSubscription(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)

	_, err = testStore.GetSubscriptionByTenant(context.Background(), reg.Tenant.ID)
	require.Error(t, err, "subscription must be deleted")
}
func TestDeleteSubscription_NonexistentTenant_NoError(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Deleting when nothing exists should not error
	err := testStore.DeleteSubscription(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
}
func TestDeleteSubscription_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	_ = createTestSubscription(t, reg1.Tenant.ID)
	_ = createTestSubscription(t, reg2.Tenant.ID)

	// Delete tenant 1's subscription
	err := testStore.DeleteSubscription(context.Background(), reg1.Tenant.ID)
	require.NoError(t, err)

	// Tenant 2's subscription must still exist
	sub2, err := testStore.GetSubscriptionByTenant(context.Background(), reg2.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, reg2.Tenant.ID, sub2.TenantID)
}

// ═══════════════════════════════════════════════════════════════
//  CreateBillingEvent
// ═══════════════════════════════════════════════════════════════

func TestCreateBillingEvent_AllFields(t *testing.T) {
	payload := json.RawMessage(`{"id":"evt_123","type":"invoice.paid","data":{"amount":2999}}`)

	stripeEventID := "evt_" + utils.RandomString(16)
	evt, err := testStore.CreateBillingEvent(context.Background(), CreateBillingEventParams{
		StripeEventID: stripeEventID,
		EventType:     "invoice.paid",
		Payload:       payload,
	})
	require.NoError(t, err)

	require.NotZero(t, evt.ID)
	require.Equal(t, stripeEventID, evt.StripeEventID)
	require.Equal(t, "invoice.paid", evt.EventType)
	require.False(t, evt.Processed)
	require.Nil(t, evt.ProcessedAt)
	require.Nil(t, evt.Error)
	require.NotZero(t, evt.CreatedAt)

	// Payload round-trip
	var expected, actual map[string]any
	require.NoError(t, json.Unmarshal(payload, &expected))
	require.NoError(t, json.Unmarshal(evt.Payload, &actual))
	require.Equal(t, expected, actual)
}

// ═══════════════════════════════════════════════════════════════
//  GetBillingEventByStripeID
// ═══════════════════════════════════════════════════════════════

func TestGetBillingEventByStripeID_Found(t *testing.T) {
	created := createTestBillingEvent(t, "customer.subscription.created")

	fetched, err := testStore.GetBillingEventByStripeID(context.Background(), created.StripeEventID)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.EventType, fetched.EventType)
}
func TestGetBillingEventByStripeID_NotFound(t *testing.T) {
	_, err := testStore.GetBillingEventByStripeID(context.Background(), "evt_does_not_exist")
	require.Error(t, err)
}

// Idempotency: same stripe_event_id should fail (unique constraint).
func TestCreateBillingEvent_DuplicateStripeID_Fails(t *testing.T) {
	stripeID := "evt_dup_" + utils.RandomString(8)

	_, err := testStore.CreateBillingEvent(context.Background(), CreateBillingEventParams{
		StripeEventID: stripeID,
		EventType:     "invoice.paid",
		Payload:       json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Second insert with same stripe_event_id must fail
	_, err = testStore.CreateBillingEvent(context.Background(), CreateBillingEventParams{
		StripeEventID: stripeID,
		EventType:     "invoice.paid",
		Payload:       json.RawMessage(`{}`),
	})
	require.Error(t, err, "duplicate stripe_event_id must be rejected")
}

// ═══════════════════════════════════════════════════════════════
//  ListUnprocessedBillingEvents
// ═══════════════════════════════════════════════════════════════

func TestListUnprocessedBillingEvents_ReturnsOnlyUnprocessed(t *testing.T) {
	evt1 := createTestBillingEvent(t, "invoice.paid")
	evt2 := createTestBillingEvent(t, "invoice.created")
	evt3 := createTestBillingEvent(t, "charge.succeeded")

	// Mark evt1 as processed
	err := testStore.MarkBillingEventProcessed(context.Background(), evt1.ID)
	require.NoError(t, err)

	unprocessed, err := testStore.ListUnprocessedBillingEvents(context.Background(), 100)
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, e := range unprocessed {
		ids[e.StripeEventID] = true
		require.False(t, e.Processed, "listed events must be unprocessed")
	}
	require.False(t, ids[evt1.StripeEventID], "processed event must not appear")
	require.True(t, ids[evt2.StripeEventID])
	require.True(t, ids[evt3.StripeEventID])
}
func TestListUnprocessedBillingEvents_RespectsLimit(t *testing.T) {
	for i := 0; i < 5; i++ {
		createTestBillingEvent(t, "invoice.paid")
	}

	events, err := testStore.ListUnprocessedBillingEvents(context.Background(), 2)
	require.NoError(t, err)
	require.LessOrEqual(t, len(events), 2)
}

func TestListUnprocessedBillingEvents_OrderByCreatedAtASC(t *testing.T) {
	for range 3 {
		createTestBillingEvent(t, "order.test")
	}

	events, err := testStore.ListUnprocessedBillingEvents(context.Background(), 100)
	require.NoError(t, err)

	for i := 1; i < len(events); i++ {
		require.True(t, events[i-1].CreatedAt.Before(events[i].CreatedAt) ||
			events[i-1].CreatedAt.Equal(events[i].CreatedAt),
			"events must be ordered by created_at ASC")
	}
}
func TestMarkBillingEventProcessed_SetsFlags(t *testing.T) {
	evt := createTestBillingEvent(t, "invoice.paid")
	require.False(t, evt.Processed)

	err := testStore.MarkBillingEventProcessed(context.Background(), evt.ID)
	require.NoError(t, err)

	fetched, err := testStore.GetBillingEventByStripeID(context.Background(), evt.StripeEventID)
	require.NoError(t, err)
	require.True(t, fetched.Processed)
	require.NotNil(t, fetched.ProcessedAt)
	require.Nil(t, fetched.Error, "successful processing must have nil error")
}

// ═══════════════════════════════════════════════════════════════
//  MarkBillingEventFailed
// ═══════════════════════════════════════════════════════════════

func TestMarkBillingEventFailed_SetsErrorAndProcessed(t *testing.T) {
	evt := createTestBillingEvent(t, "charge.failed")

	errMsg := "webhook timeout after 30s"
	err := testStore.MarkBillingEventFailed(context.Background(), MarkBillingEventFailedParams{
		ID:    evt.ID,
		Error: strPtr(errMsg),
	})
	require.NoError(t, err)

	fetched, err := testStore.GetBillingEventByStripeID(context.Background(), evt.StripeEventID)
	require.NoError(t, err)
	require.True(t, fetched.Processed)
	require.NotNil(t, fetched.ProcessedAt)
	require.NotNil(t, fetched.Error)
	require.Equal(t, errMsg, *fetched.Error)
}
func TestMarkBillingEventFailed_NilError(t *testing.T) {
	evt := createTestBillingEvent(t, "charge.failed")

	err := testStore.MarkBillingEventFailed(context.Background(), MarkBillingEventFailedParams{
		ID:    evt.ID,
		Error: nil,
	})
	require.NoError(t, err)

	fetched, err := testStore.GetBillingEventByStripeID(context.Background(), evt.StripeEventID)
	require.NoError(t, err)
	require.True(t, fetched.Processed)
	require.Nil(t, fetched.Error)
}

// ═══════════════════════════════════════════════════════════════
//  MarkBillingEventProcessed then Failed — processed stays true
// ═══════════════════════════════════════════════════════════════

func TestMarkBillingEvent_ProcessedThenFailed_StaysProcessed(t *testing.T) {
	evt := createTestBillingEvent(t, "invoice.paid")

	// Mark processed first
	err := testStore.MarkBillingEventProcessed(context.Background(), evt.ID)
	require.NoError(t, err)

	// Then mark failed — processed should still be true
	err = testStore.MarkBillingEventFailed(context.Background(), MarkBillingEventFailedParams{
		ID:    evt.ID,
		Error: strPtr("late failure"),
	})
	require.NoError(t, err)

	fetched, err := testStore.GetBillingEventByStripeID(context.Background(), evt.StripeEventID)
	require.NoError(t, err)
	require.True(t, fetched.Processed)
	require.NotNil(t, fetched.Error)
}

// ═══════════════════════════════════════════════════════════════
//  Subscription + BillingEvent — integration scenario
// ═══════════════════════════════════════════════════════════════

func TestSubscriptionLifecycle_CreateUpdateDelete(t *testing.T) {
	reg := createRegisteredTenant(t)
	now := time.Now().UTC().Truncate(time.Second)
	cleanupSubscription(t, reg.Tenant.ID)
	// 1. Create
	sub, err := testStore.CreateSubscription(context.Background(), CreateSubscriptionParams{
		TenantID:             reg.Tenant.ID,
		StripeCustomerID:     strPtr("cus_lifecycle"),
		StripeSubscriptionID: strPtr("sub_lifecycle"),
		StripePriceID:        strPtr("price_starter"),
		Plan:                 "starter",
		Status:               "active",
		CurrentPeriodStart:   timePtr(now),
		CurrentPeriodEnd:     timePtr(now.Add(30 * 24 * time.Hour)),
	})
	require.NoError(t, err)
	require.Equal(t, "starter", sub.Plan)

	// 2. Upgrade plan
	upgraded, err := testStore.UpdateSubscriptionStatus(context.Background(), UpdateSubscriptionStatusParams{
		TenantID:           reg.Tenant.ID,
		Status:             "active",
		Plan:               "enterprise",
		StripePriceID:      strPtr("price_enterprise"),
		CurrentPeriodStart: timePtr(now),
		CurrentPeriodEnd:   timePtr(now.Add(30 * 24 * time.Hour)),
		CancelAtPeriodEnd:  false,
	})
	require.NoError(t, err)
	require.Equal(t, "enterprise", upgraded.Plan)
	require.Equal(t, "price_enterprise", *upgraded.StripePriceID)

	// 3. Cancel at period end
	canceled, err := testStore.UpdateSubscriptionStatus(context.Background(), UpdateSubscriptionStatusParams{
		TenantID:           reg.Tenant.ID,
		Status:             "active",
		Plan:               "enterprise",
		StripePriceID:      strPtr("price_enterprise"),
		CurrentPeriodStart: timePtr(now),
		CurrentPeriodEnd:   timePtr(now.Add(30 * 24 * time.Hour)),
		CancelAtPeriodEnd:  true,
	})
	require.NoError(t, err)
	require.True(t, canceled.CancelAtPeriodEnd)

	// 4. Delete
	err = testStore.DeleteSubscription(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)

	_, err = testStore.GetSubscriptionByTenant(context.Background(), reg.Tenant.ID)
	require.Error(t, err)
}
