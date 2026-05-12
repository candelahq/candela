All multitenancy observability hardening tasks are now complete and verified.

### **Fixes & Improvements**
*   **W3C Baggage Compliance**: Implemented robust, case-insensitive baggage parsing with support for multiple header instances and 'right-most win' precedence.
*   **Storage Parity**: Updated BigQuery and DuckDB to match the multitenant filtering logic. Added `tenant_id` to BigQuery trace summaries.
*   **Performance**: Added a composite index (project_id, tenant_id, start_time) to SQLite for performant leaderboard queries.
*   **Pagination**: Implemented offset-based PageToken support for BigQuery and SQLite.
*   **Testing**: Added 8 unit tests and 4 integration tests (including W3C precedence and race-free in-memory SQLite isolation).
*   **Local Management**: Enabled tenant visibility in the candela-local UI handlers.

Tests passed locally with `nix develop -c go test ./...`. Ready for final review and merge.
