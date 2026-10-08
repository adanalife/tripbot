CREATE TABLE feature_flags (
    key                   TEXT PRIMARY KEY,
    description           TEXT NOT NULL,
    enabled               BOOLEAN NOT NULL DEFAULT FALSE,
    enabled_for_usernames TEXT[] NOT NULL DEFAULT '{}',
    enabled_for_roles     TEXT[] NOT NULL DEFAULT '{}',
    target_removal_date   DATE NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX feature_flags_target_removal_date_idx
    ON feature_flags (target_removal_date);
