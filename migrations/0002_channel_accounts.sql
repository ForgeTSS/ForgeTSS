-- migrations/0002_channel_accounts.sql

CREATE TABLE channel_accounts (
    public_key TEXT PRIMARY KEY,
    encrypted_secret TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'idle',
    sequence_number BIGINT NOT NULL,
    last_used_at TIMESTAMPTZ
);
