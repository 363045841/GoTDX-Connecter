package api

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const symbolDirectorySchema = `
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
);`

const sqliteBusyTimeout = 5 * time.Second

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

func newSQLiteSymbolDirectoryStore(path string) (*sqliteSymbolDirectoryStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create symbol database directory: %w", err)
	}
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)", path, sqliteBusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open symbol database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(symbolDirectorySchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize symbol database: %w", err)
	}
	return &sqliteSymbolDirectoryStore{db: db}, nil
}

func (s *sqliteSymbolDirectoryStore) Load() (symbolDirectorySnapshot, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return symbolDirectorySnapshot{}, false, fmt.Errorf("begin symbol directory read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var loadedAtUnix int64
	err = tx.QueryRow(`SELECT loaded_at_unix FROM symbol_directory_metadata WHERE id = 1`).Scan(&loadedAtUnix)
	if err == sql.ErrNoRows {
		return symbolDirectorySnapshot{}, false, nil
	}
	if err != nil {
		return symbolDirectorySnapshot{}, false, fmt.Errorf("read symbol directory metadata: %w", err)
	}

	rows, err := tx.Query(`
		SELECT source, param_kind, param_value, symbol, description, exchange
		FROM symbol_directory
		ORDER BY rowid`)
	if err != nil {
		return symbolDirectorySnapshot{}, false, fmt.Errorf("read symbol directory: %w", err)
	}
	entries := make([]symbolSearchItem, 0)
	for rows.Next() {
		var item symbolSearchItem
		var paramKind string
		var paramValue uint8
		if err := rows.Scan(&item.Source, &paramKind, &paramValue, &item.Symbol, &item.Description, &item.Exchange); err != nil {
			_ = rows.Close()
			return symbolDirectorySnapshot{}, false, fmt.Errorf("scan symbol directory: %w", err)
		}
		item.Params = map[string]uint8{paramKind: paramValue}
		entries = append(entries, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return symbolDirectorySnapshot{}, false, fmt.Errorf("iterate symbol directory: %w", err)
	}
	if err := rows.Close(); err != nil {
		return symbolDirectorySnapshot{}, false, fmt.Errorf("close symbol directory rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return symbolDirectorySnapshot{}, false, fmt.Errorf("commit symbol directory read: %w", err)
	}
	return symbolDirectorySnapshot{
		Entries:  entries,
		LoadedAt: time.Unix(loadedAtUnix, 0).UTC(),
	}, true, nil
}

func (s *sqliteSymbolDirectoryStore) Replace(snapshot symbolDirectorySnapshot) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin symbol directory replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM symbol_directory`); err != nil {
		return fmt.Errorf("clear symbol directory: %w", err)
	}
	stmt, err := tx.Prepare(`
		INSERT INTO symbol_directory
			(source, param_kind, param_value, symbol, description, exchange)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare symbol directory insert: %w", err)
	}
	defer stmt.Close()

	for _, item := range snapshot.Entries {
		paramKind, paramValue, err := symbolDirectoryParam(item.Params)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(item.Source, paramKind, paramValue, item.Symbol, item.Description, item.Exchange); err != nil {
			return fmt.Errorf("insert symbol %s: %w", item.Symbol, err)
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO symbol_directory_metadata (id, loaded_at_unix)
		VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET loaded_at_unix = excluded.loaded_at_unix`, snapshot.LoadedAt.Unix()); err != nil {
		return fmt.Errorf("update symbol directory metadata: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit symbol directory replacement: %w", err)
	}
	return nil
}

func symbolDirectoryParam(params map[string]uint8) (string, uint8, error) {
	if len(params) != 1 {
		return "", 0, fmt.Errorf("symbol directory params must contain exactly one entry")
	}
	for kind, value := range params {
		if kind != "market" && kind != "category" {
			return "", 0, fmt.Errorf("unsupported symbol directory param %q", kind)
		}
		return kind, value, nil
	}
	return "", 0, fmt.Errorf("symbol directory params are empty")
}

func (s *sqliteSymbolDirectoryStore) Close() error {
	return s.db.Close()
}
