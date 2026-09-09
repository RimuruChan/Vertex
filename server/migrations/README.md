# Development baseline

The project currently has no production deployment. `000001_init.up.sql` is the
complete fresh-install schema, including announcement publication/pinning and
the contest metadata visibility setting. Its down migration removes the schema
in reverse dependency order, without `CASCADE`.

Tables, constraints, indexes and triggers are grouped by module. Cross-table
cycles remain explicit `ALTER TABLE` statements; bootstrap domain/role data is
at the end. Update SQL inputs and regenerate sqlc after changing the baseline.

For an existing development database that already applied the former migrations
002 and 003, do not rerun 001 or roll it back. Back up the database, compare its
schema with a fresh baseline database, and only then reset the migration version
record to 1 without changing business data. Fresh databases need no conversion.

Once a production deployment exists, keep this baseline immutable and use new
numbered migrations for subsequent changes.
