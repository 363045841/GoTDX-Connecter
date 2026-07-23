# SQLite Symbol Directory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist the gotdx symbol directory in SQLite so fresh snapshots survive tdx-api restarts and are refreshed at most once every 24 hours.

**Architecture:** A focused SQLite store owns schema creation and atomic snapshot replacement. `symbolDirectoryCache` reads that store once per process, keeps the existing in-memory search behavior, and treats persistence errors as non-fatal. The router constructs the production store while tests inject fake or temporary stores.

**Tech Stack:** Go 1.26, `database/sql`, `modernc.org/sqlite`, Gin, standard `testing`

## Global Constraints

- Default database path: `data/tdx-symbols.db` relative to the process working directory.
- `SYMBOL_DB_PATH` overrides the default database path.
- Snapshot TTL: exactly 24 hours.
- Refresh retry delay after gotdx failure: exactly one minute.
- Keep existing in-memory matching, ranking, deduplication, response metadata, and limit behavior.
- SQLite failures must not make search less available than the current memory-only implementation.
- Use a pure-Go SQLite driver; do not add a CGO requirement.
- Do not add a refresh endpoint or SQLite FTS.
- Do not create commits unless the user explicitly authorizes them.

---

### Task 1: SQLite Snapshot Store

**Files:**
- Create: `services/tdx-api/internal/api/symbol_store.go`
- Create: `services/tdx-api/internal/api/symbol_store_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: existing `symbolSearchItem` from `search.go`.
- Produces: `symbolDirectorySnapshot`, `symbolDirectoryStore`, and `newSQLiteSymbolDirectoryStore(path string) (*sqliteSymbolDirectoryStore, error)`.

- [ ] **Step 1: Add the SQLite dependency**

Run:

```powershell
go get modernc.org/sqlite
```

Expected: `go.mod` contains a direct `modernc.org/sqlite` requirement and `go.sum` contains its checksums.

- [ ] **Step 2: Write failing round-trip and replacement tests**

Create `symbol_store_test.go` with tests that open `filepath.Join(t.TempDir(), "symbols.db")`, verify an empty store returns `found == false`, replace a snapshot, close and reopen the database, and compare all fields and `loadedAt`. A second replacement must completely remove rows from the first snapshot.

```go
func TestSQLiteSymbolDirectoryStorePersistsAndReplacesSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbols.db")
	store, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	if _, found, err := store.Load(); err != nil || found {
		t.Fatalf("empty Load() = found:%v err:%v", found, err)
	}

	first := symbolDirectorySnapshot{
		Entries: []symbolSearchItem{{
			Symbol: "000001", Description: "Ping An", Exchange: "SZ", Source: "gotdx",
			Params: map[string]uint8{"market": 0},
		}},
		LoadedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
	if err := store.Replace(first); err != nil {
		t.Fatalf("replace first snapshot: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	store, err = newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	got, found, err := store.Load()
	if err != nil || !found || !reflect.DeepEqual(got, first) {
		t.Fatalf("Load() = %#v, found:%v err:%v, want %#v", got, found, err, first)
	}

	second := symbolDirectorySnapshot{
		Entries: []symbolSearchItem{{
			Symbol: "00700", Description: "Tencent", Exchange: "HK", Source: "gotdx",
			Params: map[string]uint8{"category": 7},
		}},
		LoadedAt: first.LoadedAt.Add(time.Hour),
	}
	if err := store.Replace(second); err != nil {
		t.Fatalf("replace second snapshot: %v", err)
	}
	got, found, err = store.Load()
	if err != nil || !found || !reflect.DeepEqual(got, second) {
		t.Fatalf("Load() after replacement = %#v, found:%v err:%v", got, found, err)
	}
}
```

Add a rollback test that creates a trigger after writing a valid snapshot, forces the next insert to abort, and verifies the first snapshot is unchanged:

```go
func TestSQLiteSymbolDirectoryStorePreservesSnapshotWhenReplacementFails(t *testing.T) {
	store, err := newSQLiteSymbolDirectoryStore(filepath.Join(t.TempDir(), "symbols.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first := symbolDirectorySnapshot{
		Entries: []symbolSearchItem{{
			Symbol: "000001", Description: "Ping An", Exchange: "SZ", Source: "gotdx",
			Params: map[string]uint8{"market": 0},
		}},
		LoadedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
	if err := store.Replace(first); err != nil {
		t.Fatalf("replace first snapshot: %v", err)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER reject_failed_symbol
		BEFORE INSERT ON symbol_directory
		WHEN NEW.symbol = 'FAIL'
		BEGIN SELECT RAISE(ABORT, 'forced failure'); END
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	failed := symbolDirectorySnapshot{
		Entries: []symbolSearchItem{{
			Symbol: "FAIL", Description: "Failure", Exchange: "SZ", Source: "gotdx",
			Params: map[string]uint8{"market": 0},
		}},
		LoadedAt: first.LoadedAt.Add(time.Hour),
	}
	if err := store.Replace(failed); err == nil {
		t.Fatal("Replace() error = nil, want forced failure")
	}
	got, found, err := store.Load()
	if err != nil || !found || !reflect.DeepEqual(got, first) {
		t.Fatalf("Load() after rollback = %#v, found:%v err:%v, want %#v", got, found, err, first)
	}
}
```

- [ ] **Step 3: Run the store test and verify it fails**

Run:

```powershell
go test ./services/tdx-api/internal/api -run TestSQLiteSymbolDirectoryStorePersistsAndReplacesSnapshot -count=1
```

Expected: FAIL because `newSQLiteSymbolDirectoryStore` and `symbolDirectorySnapshot` do not exist.

- [ ] **Step 4: Implement the store and schema**

Create `symbol_store.go` with these exact boundaries:

```go
type symbolDirectorySnapshot struct {
	Entries  []symbolSearchItem
	LoadedAt time.Time
}

type symbolDirectoryStore interface {
	Load() (symbolDirectorySnapshot, bool, error)
	Replace(snapshot symbolDirectorySnapshot) error
}

type sqliteSymbolDirectoryStore struct {
	db *sql.DB
}

func newSQLiteSymbolDirectoryStore(path string) (*sqliteSymbolDirectoryStore, error)
func (s *sqliteSymbolDirectoryStore) Load() (symbolDirectorySnapshot, bool, error)
func (s *sqliteSymbolDirectoryStore) Replace(snapshot symbolDirectorySnapshot) error
func (s *sqliteSymbolDirectoryStore) Close() error
```

`newSQLiteSymbolDirectoryStore` creates the parent directory with mode `0755`, opens driver name `sqlite`, enables a single connection with `db.SetMaxOpenConns(1)`, and executes:

```sql
CREATE TABLE IF NOT EXISTS symbol_directory (
    source TEXT NOT NULL,
    param_kind TEXT NOT NULL CHECK (param_kind IN ('market', 'category')),
    param_value INTEGER NOT NULL CHECK (param_value BETWEEN 0 AND 255),
    symbol TEXT NOT NULL,
    description TEXT NOT NULL,
    exchange TEXT NOT NULL,
    PRIMARY KEY (source, param_kind, param_value, symbol)
);
CREATE TABLE IF NOT EXISTS symbol_directory_metadata (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    loaded_at_unix INTEGER NOT NULL
);
```

`Replace` validates that every item has exactly one supported params entry, then starts a transaction, deletes directory rows, inserts each row, and upserts metadata id `1`. It commits only after all operations succeed. `Load` reads metadata first; absent metadata means `found == false`. It then orders rows by `rowid`, reconstructs `map[string]uint8{paramKind: uint8(paramValue)}`, and returns `time.Unix(loadedAtUnix, 0).UTC()`.

- [ ] **Step 5: Run store tests**

Run:

```powershell
go test ./services/tdx-api/internal/api -run SQLiteSymbolDirectoryStore -count=1
```

Expected: PASS.

---

### Task 2: Cache Persistence And Graceful Degradation

**Files:**
- Modify: `services/tdx-api/internal/api/search.go`
- Modify: `services/tdx-api/internal/api/search_test.go`
- Modify: `services/tdx-api/internal/api/router.go`

**Interfaces:**
- Consumes: `symbolDirectoryStore.Load` and `symbolDirectoryStore.Replace` from Task 1.
- Produces: `newSymbolDirectoryCache(loader symbolDirectoryLoader, store symbolDirectoryStore, ttl time.Duration)` and `newRouter(cache *symbolDirectoryCache)` for test injection.

- [ ] **Step 1: Add a fake store and failing cache reuse tests**

Add a fake store whose `Load` and `Replace` calls and errors are observable. Update `newSearchTestCache` to pass a store argument. Add these assertions:

```go
func TestSymbolDirectoryCacheUsesFreshPersistedSnapshot(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	loader := &fakeSymbolDirectoryLoader{err: errors.New("loader must not be called")}
	store := &fakeSymbolDirectoryStore{snapshot: symbolDirectorySnapshot{
		Entries: []symbolSearchItem{{
			Symbol: "000001", Description: "Ping An", Exchange: "SZ", Source: "gotdx",
			Params: map[string]uint8{"market": 0},
		}},
		LoadedAt: now.Add(-time.Hour),
	}, found: true}
	cache := newSymbolDirectoryCache(loader, store, 24*time.Hour)
	cache.now = func() time.Time { return now }

	items, err := cache.search("000001", 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("search = %#v, err:%v", items, err)
	}
	if loader.stockCalls != 0 || store.loadCalls != 1 || store.replaceCalls != 0 {
		t.Fatalf("calls = loader:%d load:%d replace:%d", loader.stockCalls, store.loadCalls, store.replaceCalls)
	}
}
```

Also add tests proving that a stale persisted snapshot is returned when refresh fails, a successful refresh calls `Replace`, and store read/write errors still permit loader-backed search.

- [ ] **Step 2: Run cache persistence tests and verify failure**

Run:

```powershell
go test ./services/tdx-api/internal/api -run 'TestSymbolDirectoryCache(UsesFreshPersistedSnapshot|UsesStalePersistedSnapshot|PersistsRefresh|IgnoresStore)' -count=1
```

Expected: FAIL because the cache constructor does not accept a store and does not load snapshots.

- [ ] **Step 3: Integrate persisted snapshots into the cache**

Change the TTL constant and cache fields:

```go
const symbolDirectoryTTL = 24 * time.Hour

type symbolDirectoryCache struct {
	loader    symbolDirectoryLoader
	store     symbolDirectoryStore
	ttl       time.Duration
	now       func() time.Time
	mu        sync.Mutex
	entries   []symbolSearchItem
	loadedAt  time.Time
	retryAt   time.Time
	loaded    bool
	storeRead bool
}
```

At the start of `directory`, call a locked helper once per cache instance:

```go
func (c *symbolDirectoryCache) loadPersistedDirectory() {
	if c.storeRead || c.store == nil {
		return
	}
	c.storeRead = true
	snapshot, found, err := c.store.Load()
	if err != nil {
		log.Printf("symbol directory database read failed: %v", err)
		return
	}
	if found {
		c.entries = snapshot.Entries
		c.loadedAt = snapshot.LoadedAt
		c.loaded = true
	}
}
```

After a successful gotdx load, attempt persistence before assigning memory state. A `Replace` error is logged but not returned:

```go
snapshot := symbolDirectorySnapshot{Entries: entries, LoadedAt: now}
if c.store != nil {
	if err := c.store.Replace(snapshot); err != nil {
		log.Printf("symbol directory database write failed: %v", err)
	}
}
c.entries = entries
c.loadedAt = now
```

Keep the current stale fallback and one-minute retry behavior unchanged.

- [ ] **Step 4: Inject the cache into router construction**

Make `NewRouter` initialize the production store from `SYMBOL_DB_PATH`, falling back to `data/tdx-symbols.db`. If store creation fails, log and pass `nil` so search remains memory-only. Move route setup into `newRouter(cache *symbolDirectoryCache)` and register `newSymbolSearchHandler(cache)` directly. Tests use `newRouter(testCache)` and never create a repository-local database.

```go
func NewRouter() *gin.Engine {
	path := os.Getenv("SYMBOL_DB_PATH")
	if path == "" {
		path = filepath.Join("data", "tdx-symbols.db")
	}
	store, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		log.Printf("symbol directory database disabled: %v", err)
		store = nil
	}
	return newRouter(newSymbolDirectoryCache(gotdxSymbolDirectoryLoader{}, store, symbolDirectoryTTL))
}
```

Remove the package-global `defaultSymbolDirectoryCache` and `handleSymbolSearch` wrapper after the route uses the injected handler.

- [ ] **Step 5: Run all API package tests**

Run:

```powershell
go test ./services/tdx-api/internal/api -count=1
```

Expected: PASS, including all pre-existing search ranking and handler tests.

---

### Task 3: Runtime Configuration And Full Verification

**Files:**
- Modify: `.gitignore`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: `SYMBOL_DB_PATH` and default `data/tdx-symbols.db` from Task 2.
- Produces: documented runtime configuration and protection against committing generated database files.

- [ ] **Step 1: Ignore runtime SQLite data**

Append this exact rule to `.gitignore`:

```gitignore
/data/
```

- [ ] **Step 2: Document persistence configuration**

Add `SYMBOL_DB_PATH` to the tdx-api environment variable table in `AGENTS.md` with default `data/tdx-symbols.db`. Replace the stale “no DB, no tests” key fact with text stating that symbol search uses SQLite persistence and API package tests exist.

- [ ] **Step 3: Format changed Go files**

Run:

```powershell
gofmt -w services/tdx-api/internal/api/search.go services/tdx-api/internal/api/search_test.go services/tdx-api/internal/api/symbol_store.go services/tdx-api/internal/api/symbol_store_test.go services/tdx-api/internal/api/router.go
```

Expected: command exits 0 and produces no output.

- [ ] **Step 4: Run complete tests**

Run:

```powershell
go test ./...
```

Expected: PASS for every package.

- [ ] **Step 5: Run static analysis**

Run:

```powershell
go vet ./...
```

Expected: exits 0 with no diagnostics.

- [ ] **Step 6: Inspect the final worktree**

Run:

```powershell
git diff --check
```

Expected: no whitespace errors; only the approved SQLite implementation, tests, dependency files, docs, and ignore rule are changed. Do not commit without explicit user authorization.
