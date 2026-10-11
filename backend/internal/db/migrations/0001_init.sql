-- 0001_init.sql — initial OpenHandle schema.

-- ---------- users ----------
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL,
    password_hash text NOT NULL,
    status        text NOT NULL DEFAULT 'active', -- active | deleted
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
-- Case-insensitive uniqueness for login identifiers.
CREATE UNIQUE INDEX users_email_lower_uniq ON users (lower(email));

-- ---------- sessions ----------
CREATE TABLE sessions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE, -- sha256(token); raw token only lives in the cookie
    csrf_token  text NOT NULL,
    user_agent  text NOT NULL DEFAULT '',
    ip          text NOT NULL DEFAULT '',
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- ---------- handles ----------
CREATE TABLE handles (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    handle       text NOT NULL,
    display_name text,
    bio          text,
    is_public    boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    released_at  timestamptz
);
-- Uniqueness is enforced in the database, among active (non-released) handles.
CREATE UNIQUE INDEX handles_handle_active_uniq ON handles (handle) WHERE released_at IS NULL;
-- One active handle per user.
CREATE UNIQUE INDEX handles_user_active_uniq ON handles (user_id) WHERE released_at IS NULL;
CREATE INDEX handles_user_id_idx ON handles (user_id);

-- ---------- wallets ----------
CREATE TABLE wallets (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    network               text NOT NULL DEFAULT 'solana',
    address               text NOT NULL,
    is_primary            boolean NOT NULL DEFAULT false,
    ownership_verified_at timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
-- A given address may only belong to one account.
CREATE UNIQUE INDEX wallets_network_address_uniq ON wallets (network, address);
-- At most one primary wallet per user.
CREATE UNIQUE INDEX wallets_user_primary_uniq ON wallets (user_id) WHERE is_primary;
CREATE INDEX wallets_user_id_idx ON wallets (user_id);

-- ---------- wallet_challenges ----------
CREATE TABLE wallet_challenges (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    wallet_address text NOT NULL,
    nonce_hash     text NOT NULL, -- sha256(nonce); challenge is single-use
    domain         text NOT NULL,
    message        text NOT NULL,
    expires_at     timestamptz NOT NULL,
    consumed_at    timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wallet_challenges_user_id_idx ON wallet_challenges (user_id);
CREATE INDEX wallet_challenges_expires_at_idx ON wallet_challenges (expires_at);

-- ---------- identity_connections ----------
CREATE TABLE identity_connections (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider          text NOT NULL,
    provider_subject  text NOT NULL,
    provider_username text NOT NULL DEFAULT '',
    verified_at       timestamptz NOT NULL DEFAULT now(),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX identity_connections_provider_subject_uniq
    ON identity_connections (provider, provider_subject);
CREATE UNIQUE INDEX identity_connections_user_provider_uniq
    ON identity_connections (user_id, provider);

-- ---------- transactions ----------
-- Money is stored as integer base units (numeric to avoid overflow), never float.
CREATE TABLE transactions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    network             text NOT NULL,
    signature           text NOT NULL,
    sender_address      text,
    recipient_address   text NOT NULL,
    amount_base_units   numeric(30,0) NOT NULL,
    asset_identifier    text NOT NULL DEFAULT 'SOL',
    status              text NOT NULL DEFAULT 'submitted', -- submitted|confirmed|failed|expired
    slot                bigint,
    submitted_at        timestamptz NOT NULL DEFAULT now(),
    confirmed_at        timestamptz,
    last_checked_at     timestamptz,
    created_by_user_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    sender_handle_id    uuid REFERENCES handles(id) ON DELETE SET NULL,
    recipient_handle_id uuid REFERENCES handles(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);
-- Idempotent ingestion: a signature is unique within a network.
CREATE UNIQUE INDEX transactions_network_signature_uniq ON transactions (network, signature);
CREATE INDEX transactions_created_by_user_idx ON transactions (created_by_user_id, submitted_at DESC);
CREATE INDEX transactions_recipient_address_idx ON transactions (recipient_address);
CREATE INDEX transactions_sender_address_idx ON transactions (sender_address);

-- ---------- audit_events ----------
CREATE TABLE audit_events (
    id            bigserial PRIMARY KEY,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    event_type    text NOT NULL,
    target_type   text NOT NULL DEFAULT '',
    target_id     text NOT NULL DEFAULT '',
    metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_user_id, created_at DESC);
CREATE INDEX audit_events_type_idx ON audit_events (event_type, created_at DESC);
