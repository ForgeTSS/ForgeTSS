-- migrations/0001_init.sql

CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    envelope_xdr TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    channel_account TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
