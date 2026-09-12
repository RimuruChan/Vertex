# Development baseline

The project currently has no production deployment. `000001_init.up.sql` is the
complete fresh-install schema, including announcement publication/pinning,
contest formats (`icpc`, `ioi`, `oi`, `leduo`, `cf`) and feedback options
(`full`, `summary`, `first_error`, `none`). Its down migration removes the schema
in reverse dependency order, without `CASCADE`.

Tables, constraints, indexes and triggers are grouped by module. Cross-table
cycles remain explicit `ALTER TABLE` statements; bootstrap domain/role data is
at the end. Update SQL inputs and regenerate sqlc after changing the baseline.

During development, fold schema changes directly into this baseline instead of
adding incremental migrations. Validate changes against a fresh, isolated
database; an existing database at version 1 is not automatically upgraded.

Once a production deployment exists, keep this baseline immutable and use new
numbered migrations for subsequent changes.
