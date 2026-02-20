-- Section 4: Billing (Stripe)

CREATE TABLE subscriptions (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              UUID        NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    stripe_customer_id     TEXT        UNIQUE,
    stripe_subscription_id TEXT        UNIQUE,
    stripe_price_id        TEXT,
    plan                   TEXT        NOT NULL DEFAULT 'cloud',
    status                 TEXT        NOT NULL DEFAULT 'trialing',
    current_period_start   TIMESTAMPTZ,
    current_period_end     TIMESTAMPTZ,
    cancel_at_period_end   BOOLEAN     NOT NULL DEFAULT false,
    trial_end              TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE billing_events (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    stripe_event_id TEXT        NOT NULL UNIQUE,
    event_type      TEXT        NOT NULL,
    payload         JSONB       NOT NULL,
    processed       BOOLEAN     NOT NULL DEFAULT false,
    processed_at    TIMESTAMPTZ,
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_billing_events_unprocessed ON billing_events(processed) WHERE processed = false;