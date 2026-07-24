package api

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

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
			Params: map[string]any{"market": uint8(0), "kind": "stock"},
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
			Params: map[string]any{"category": uint8(7), "kind": "ex"},
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

func TestSQLiteSymbolDirectoryStorePreservesSnapshotWhenReplacementFails(t *testing.T) {
	store, err := newSQLiteSymbolDirectoryStore(filepath.Join(t.TempDir(), "symbols.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first := symbolDirectorySnapshot{
		Entries: []symbolSearchItem{{
			Symbol: "000001", Description: "Ping An", Exchange: "SZ", Source: "gotdx",
			Params: map[string]any{"market": uint8(0), "kind": "stock"},
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
			Params: map[string]any{"market": uint8(0), "kind": "stock"},
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

func TestSQLiteSymbolDirectoryStoreWaitsForConcurrentWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbols.db")
	firstStore, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	t.Cleanup(func() { _ = firstStore.Close() })
	secondStore, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	t.Cleanup(func() { _ = secondStore.Close() })

	tx, err := firstStore.db.Begin()
	if err != nil {
		t.Fatalf("begin blocking transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO symbol_directory_metadata (id, loaded_at_unix) VALUES (1, 1)`); err != nil {
		t.Fatalf("acquire write lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- secondStore.Replace(symbolDirectorySnapshot{
			Entries: []symbolSearchItem{{
				Symbol: "000001", Description: "Ping An", Exchange: "SZ", Source: "gotdx",
				Params: map[string]any{"market": uint8(0), "kind": "stock"},
			}},
			LoadedAt: time.Unix(2, 0).UTC(),
		})
	}()

	select {
	case err := <-done:
		t.Fatalf("Replace() returned before lock release: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("release write lock: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Replace() after lock release: %v", err)
	}
}

func TestSQLiteSymbolDirectoryStoreLoadsConsistentSnapshotDuringReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbols.db")
	reader, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	writer, err := newSQLiteSymbolDirectoryStore(path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	snapshots := []symbolDirectorySnapshot{
		{
			Entries: []symbolSearchItem{{
				Symbol: "A", Description: "Version A", Exchange: "SZ", Source: "gotdx",
				Params: map[string]any{"market": uint8(0), "kind": "stock"},
			}},
			LoadedAt: time.Unix(1, 0).UTC(),
		},
		{
			Entries: []symbolSearchItem{{
				Symbol: "B", Description: "Version B", Exchange: "SH", Source: "gotdx",
				Params: map[string]any{"market": uint8(1), "kind": "index"},
			}},
			LoadedAt: time.Unix(2, 0).UTC(),
		},
	}
	if err := writer.Replace(snapshots[0]); err != nil {
		t.Fatalf("write initial snapshot: %v", err)
	}

	stop := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				writerDone <- nil
				return
			default:
			}
			if err := writer.Replace(snapshots[i%len(snapshots)]); err != nil {
				writerDone <- err
				return
			}
		}
	}()

	var readErr error
	for range 200 {
		snapshot, found, err := reader.Load()
		if err != nil {
			readErr = err
			break
		}
		if !found || len(snapshot.Entries) != 1 {
			readErr = fmt.Errorf("invalid snapshot: %#v, found:%v", snapshot, found)
			break
		}
		wantSymbol := map[int64]string{1: "A", 2: "B"}[snapshot.LoadedAt.Unix()]
		if snapshot.Entries[0].Symbol != wantSymbol {
			readErr = fmt.Errorf("mixed snapshot: loadedAt:%s entry:%#v", snapshot.LoadedAt, snapshot.Entries[0])
			break
		}
	}
	close(stop)
	if err := <-writerDone; err != nil {
		t.Fatalf("replace snapshots: %v", err)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
}
