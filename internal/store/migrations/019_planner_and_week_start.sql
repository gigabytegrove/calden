-- Planner permissions and account-level week start.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('admin','planner','member','restricted'));
ALTER TABLE users ADD COLUMN week_start TEXT
    CONSTRAINT users_week_start_check CHECK (week_start IN ('sunday','monday'));
