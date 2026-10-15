-- +goose Up
ALTER TABLE payments DROP CONSTRAINT payments_kind_check;
ALTER TABLE payments ADD CONSTRAINT payments_kind_check CHECK(kind IN ('new','renew','package','trial','device'));
ALTER TABLE payments DROP CONSTRAINT payments_check;
ALTER TABLE payments ADD CONSTRAINT payments_check CHECK(kind IN ('package','device') OR tariff_id IS NOT NULL);
CREATE TABLE cg_referral_codes (
 user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 code TEXT NOT NULL UNIQUE
);
CREATE TABLE cg_referrals (
 invitee BIGINT PRIMARY KEY,
 referrer BIGINT REFERENCES users(id) ON DELETE SET NULL,
 created_at BIGINT NOT NULL,
 payment_id BIGINT UNIQUE REFERENCES payments(id) ON DELETE SET NULL,
 seconds BIGINT NOT NULL DEFAULT 0,
 reversed BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE cg_device_grants (
 payment_id BIGINT PRIMARY KEY REFERENCES payments(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX cg_device_grants_user ON cg_device_grants(user_id);
CREATE TABLE cg_external_sources (
 id BIGSERIAL PRIMARY KEY,
 name TEXT NOT NULL,
 url TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 tariff_ids TEXT NOT NULL DEFAULT '[]',
 interval_minutes BIGINT NOT NULL DEFAULT 60 CHECK(interval_minutes BETWEEN 5 AND 10080),
 body TEXT NOT NULL DEFAULT '',
 fetched_at BIGINT NOT NULL DEFAULT 0,
 attempted_at BIGINT NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT ''
);
-- +goose Down
ALTER TABLE payments DROP CONSTRAINT payments_kind_check;
ALTER TABLE payments ADD CONSTRAINT payments_kind_check CHECK(kind IN ('new','renew','package','trial')) NOT VALID;
ALTER TABLE payments DROP CONSTRAINT payments_check;
ALTER TABLE payments ADD CONSTRAINT payments_check CHECK(kind='package' OR tariff_id IS NOT NULL) NOT VALID;
DROP TABLE cg_external_sources;
DROP TABLE cg_device_grants;
DROP TABLE cg_referrals;
DROP TABLE cg_referral_codes;
