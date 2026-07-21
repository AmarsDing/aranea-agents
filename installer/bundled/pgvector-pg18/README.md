# Vendored pgvector for Windows portable Postgres 18.x

These files must match the **major version** of the bundled PostgreSQL
downloaded by `scripts/download-deps.ps1` (currently 18.3).

| Path | Purpose |
|------|---------|
| `lib/vector.dll` | Extension shared library |
| `share/extension/vector.control` | Extension control file |
| `share/extension/vector*.sql` | Extension SQL scripts |

Source: built against PostgreSQL 18.3 (pgvector 0.8.2).

Do not mix with PostgreSQL 17 binaries — ABI mismatch will fail `CREATE EXTENSION vector`.
