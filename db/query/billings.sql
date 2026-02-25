-- name: CreateSubscription :one
INSERT INTO subscriptions (
    tenant_id,
    stripe_customer_id,
    stripe_subscription_id,
    stripe_price_id,
    plan,
    status,
    current_period_start,
    current_period_end,
    trial_end
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetSubscriptionByTenant :one
SELECT * FROM subscriptions
WHERE tenant_id = $1
LIMIT 1;

-- name: GetSubscriptionByStripeID :one
SELECT * FROM subscriptions
WHERE stripe_subscription_id = $1
LIMIT 1;

-- name: GetSubscriptionByStripeCustomer :one
SELECT * FROM subscriptions
WHERE stripe_customer_id = $1
LIMIT 1;

-- name: UpdateSubscriptionStatus :one
UPDATE subscriptions
SET
    status = $2,
    plan = $3,
    stripe_price_id = $4,
    current_period_start = $5,
    current_period_end = $6,
    cancel_at_period_end = $7,
    updated_at = now()
WHERE tenant_id = $1
RETURNING *;

-- name: UpdateSubscriptionStripeIDs :one
UPDATE subscriptions
SET
    stripe_customer_id = $2,
    stripe_subscription_id = $3,
    updated_at = now()
WHERE tenant_id = $1
RETURNING *;

-- name: DeleteSubscription :exec
DELETE FROM subscriptions
WHERE tenant_id = $1;

-- name: CreateBillingEvent :one
INSERT INTO billing_events (
    stripe_event_id,
    event_type,
    payload
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: GetBillingEventByStripeID :one
SELECT * FROM billing_events
WHERE stripe_event_id = $1
LIMIT 1;

-- name: MarkBillingEventProcessed :exec
UPDATE billing_events
SET
    processed = true,
    processed_at = now()
WHERE id = $1;

-- name: MarkBillingEventFailed :exec
UPDATE billing_events
SET
    processed = true,
    processed_at = now(),
    error = $2
WHERE id = $1;

-- name: ListUnprocessedBillingEvents :many
SELECT * FROM billing_events
WHERE processed = false
ORDER BY created_at ASC
LIMIT $1;