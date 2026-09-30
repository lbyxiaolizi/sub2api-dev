-- Account configuration groups are distinct from the account_groups routing bindings.
-- A member belongs to at most one configuration group and must retain its parent
-- routing-group binding, including when bindings are replaced within a transaction.
CREATE TABLE IF NOT EXISTS account_config_groups (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL CHECK (length(trim(name)) > 0),
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL,
    type VARCHAR(20) NOT NULL,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_account_config_groups_group_id ON account_config_groups(group_id);

CREATE TABLE IF NOT EXISTS account_config_group_members (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    account_config_group_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    CONSTRAINT account_config_group_members_group_fk
        FOREIGN KEY (account_config_group_id, group_id)
        REFERENCES account_config_groups(id, group_id) ON DELETE CASCADE,
    CONSTRAINT account_config_group_members_routing_fk
        FOREIGN KEY (account_id, group_id)
        REFERENCES account_groups(account_id, group_id)
        DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX IF NOT EXISTS idx_account_config_group_members_group_id
    ON account_config_group_members(account_config_group_id);
