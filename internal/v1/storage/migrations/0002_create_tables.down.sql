BEGIN;
drop schema if exists bridge cascade;
drop table public.schema_migrations;
COMMIT;