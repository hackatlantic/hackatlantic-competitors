# Backup and restore runbook

Nightly automation dumps only the private `ats` schema, encrypts the archive locally with the configured `age` recipient, and uploads ciphertext to a separate private Space. Retention is 14 daily, 8 weekly, and 6 monthly recovery points.

## Key custody

- GitHub’s `backup` environment contains the database and least-privilege Spaces write credentials.
- The public `age` recipient is safe as an environment variable.
- The private `age` identity exists only in the protected `disaster-recovery` environment and the administrators’ offline recovery material.
- Losing the private identity makes backups unrecoverable; exposing it compromises every retained backup.

## Monthly drill

1. A second administrator approves the `disaster-recovery` environment.
2. Automation starts disposable PostgreSQL 17 and downloads the newest daily ciphertext.
3. `age` decrypts locally; `pg_restore` restores the `ats` schema.
4. Forward migrations run against the restored database.
5. Integrity checks report schema, table, and migration counts only.
6. Record the measured RTO and compare it with the 30-minute target.
7. GitHub destroys the service container and runner filesystem at job completion.

If the drill fails, page administrators, preserve only non-sensitive error metadata, repair the process, and repeat the drill. Never display row contents, applicant counts grouped by private attributes, or resume metadata in logs.
