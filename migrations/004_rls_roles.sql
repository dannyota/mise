-- +goose Up

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'mise_public') THEN
    CREATE ROLE mise_public;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'mise_group') THEN
    CREATE ROLE mise_group;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'mise_local') THEN
    CREATE ROLE mise_local;
  END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA vn_reg TO mise_public, mise_group, mise_local;
GRANT USAGE ON SCHEMA my_reg TO mise_public, mise_group, mise_local;
GRANT USAGE ON SCHEMA group_std TO mise_group, mise_local;
GRANT USAGE ON SCHEMA local_policy TO mise_local;
GRANT USAGE ON SCHEMA local_sop TO mise_local;
GRANT USAGE ON SCHEMA graph TO mise_public, mise_group, mise_local;

-- +goose StatementBegin
DO $$
DECLARE
  s text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
  LOOP
    EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA %I TO mise_public, mise_group, mise_local', s);
    EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT SELECT ON TABLES TO mise_public, mise_group, mise_local', s);
    EXECUTE format('ALTER TABLE %I.document ENABLE ROW LEVEL SECURITY', s);
    EXECUTE format('ALTER TABLE %I.section ENABLE ROW LEVEL SECURITY', s);
  END LOOP;
END $$;
-- +goose StatementEnd

CREATE POLICY public_read ON vn_reg.document FOR SELECT TO mise_public
  USING (access_tier = 'public');
CREATE POLICY public_read ON vn_reg.section FOR SELECT TO mise_public
  USING (access_tier = 'public');
CREATE POLICY public_read ON my_reg.document FOR SELECT TO mise_public
  USING (access_tier = 'public');
CREATE POLICY public_read ON my_reg.section FOR SELECT TO mise_public
  USING (access_tier = 'public');

CREATE POLICY group_read ON group_std.document FOR SELECT TO mise_group
  USING (access_tier IN ('public', 'group-confidential'));
CREATE POLICY group_read ON group_std.section FOR SELECT TO mise_group
  USING (access_tier IN ('public', 'group-confidential'));

CREATE POLICY local_read ON local_policy.document FOR SELECT TO mise_local
  USING (true);
CREATE POLICY local_read ON local_policy.section FOR SELECT TO mise_local
  USING (true);
CREATE POLICY local_read ON local_sop.document FOR SELECT TO mise_local
  USING (true);
CREATE POLICY local_read ON local_sop.section FOR SELECT TO mise_local
  USING (true);

-- +goose Down

-- +goose StatementBegin
DO $$
DECLARE
  s text;
  t text;
BEGIN
  FOR s IN SELECT unnest(ARRAY['vn_reg','my_reg','group_std','local_policy','local_sop'])
  LOOP
    FOR t IN SELECT unnest(ARRAY['document','section'])
    LOOP
      EXECUTE format('ALTER TABLE IF EXISTS %I.%I DISABLE ROW LEVEL SECURITY', s, t);
    END LOOP;
  END LOOP;
END $$;
-- +goose StatementEnd

DROP POLICY IF EXISTS public_read ON vn_reg.document;
DROP POLICY IF EXISTS public_read ON vn_reg.section;
DROP POLICY IF EXISTS public_read ON my_reg.document;
DROP POLICY IF EXISTS public_read ON my_reg.section;
DROP POLICY IF EXISTS group_read ON group_std.document;
DROP POLICY IF EXISTS group_read ON group_std.section;
DROP POLICY IF EXISTS local_read ON local_policy.document;
DROP POLICY IF EXISTS local_read ON local_policy.section;
DROP POLICY IF EXISTS local_read ON local_sop.document;
DROP POLICY IF EXISTS local_read ON local_sop.section;

-- DROP ROLE fails while a role still holds privileges (schema USAGE, table
-- SELECT, default-privilege ACL entries from the grants above). DROP OWNED BY
-- clears all of those in one statement — plain REVOKEs would miss the
-- ALTER DEFAULT PRIVILEGES entries.
DROP OWNED BY mise_public, mise_group, mise_local;

DROP ROLE IF EXISTS mise_local;
DROP ROLE IF EXISTS mise_group;
DROP ROLE IF EXISTS mise_public;
