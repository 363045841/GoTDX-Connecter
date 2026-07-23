# SQLite Symbol Directory Persistence

## Goal

Persist the gotdx security directory across tdx-api restarts so symbol search does not download the full directory on every process start.

## Scope

- Persist the directory used by `POST /api/symbol/search`.
- Keep the existing in-memory matching, ranking, deduplication, and limit behavior.
- Refresh the persisted directory at most once every 24 hours.
- Keep stale data available when gotdx is temporarily unavailable.
- Do not add a manual refresh endpoint or move search execution into SQLite.

## Configuration

The default database path is `data/tdx-symbols.db`, relative to the process working directory. `SYMBOL_DB_PATH` overrides the path. The service creates the parent directory when needed.

## Architecture

Add a directory store interface between `symbolDirectoryCache` and SQLite. The SQLite implementation uses the pure-Go `modernc.org/sqlite` driver to avoid a CGO deployment dependency.

SQLite is the cross-process snapshot store. The existing cache remains the in-process search index and synchronization boundary. A fresh process loads persisted rows into memory before searching; search requests do not query SQLite for individual matches.

The database contains:

- `symbol_directory`: one row per unique gotdx directory item, including `symbol`, `description`, `exchange`, `source`, `param_kind`, and `param_value`.
- `symbol_directory_metadata`: the successful snapshot timestamp.

`param_kind` is `market` or `category`; `param_value` reconstructs the existing `params` response object without storing opaque JSON.

## Data Flow

On the first search handled by a cache instance:

1. Read the SQLite snapshot.
2. If the snapshot exists and is less than 24 hours old, load it into memory and search without calling gotdx.
3. If it is missing or expired, request the complete directory from gotdx.
4. After a successful download, replace all persisted rows and the snapshot timestamp in one transaction.
5. Update the in-memory directory and run the existing search logic.

Subsequent searches use memory until the 24-hour TTL expires. After expiry, one request performs the synchronized refresh while other requests wait on the existing cache mutex.

## Atomicity And Failure Handling

Directory replacement deletes old rows, inserts the new rows, and updates metadata in one transaction. Any failure rolls the transaction back and preserves the previous snapshot.

- If gotdx refresh fails and an in-memory or SQLite snapshot exists, return stale results and wait one minute before retrying.
- If no snapshot exists and gotdx fails, return the current HTTP 500 response.
- If SQLite cannot be opened or read, log the error and continue with gotdx plus the existing memory-only cache.
- If gotdx succeeds but the SQLite write fails, log the error, update memory, and return fresh results.
- A persistence failure must not make symbol search less available than the current implementation.

## Lifecycle

The tdx-api router initializes the default directory store and cache. Tests can inject a fake store or a temporary SQLite database. The database connection lives for the process lifetime.

## Testing

Store tests use a temporary SQLite file and cover:

- schema creation and empty reads;
- row and timestamp round trips;
- complete transactional replacement without duplicate accumulation;
- preservation of the previous snapshot after a failed replacement.

Cache tests cover:

- a fresh persisted snapshot avoids all gotdx loader calls;
- a stale snapshot refreshes through gotdx and is persisted;
- a stale snapshot is returned when gotdx refresh fails;
- a new cache instance reuses data written by a previous instance;
- read and write failures degrade to in-memory behavior;
- existing ranking, metadata, deduplication, request validation, and retry backoff remain unchanged.

Verification commands are `go test ./...` and `go vet ./...`.
