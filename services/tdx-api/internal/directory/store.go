// 证券目录 SQLite 持久化：快照读写、一致性替换与 WAL 日志模式。
package directory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS symbol_directory (
	source TEXT NOT NULL,
	param_kind TEXT NOT NULL CHECK (param_kind IN ('market', 'category')),
	param_value INTEGER NOT NULL CHECK (param_value BETWEEN 0 AND 255),
	symbol TEXT NOT NULL,
	description TEXT NOT NULL,
	exchange TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'stock',
	PRIMARY KEY (source, param_kind, param_value, symbol)
);
CREATE TABLE IF NOT EXISTS symbol_directory_metadata (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	loaded_at_unix INTEGER NOT NULL
);`

const sqliteBusyTimeout = 5 * time.Second

// SQLiteStore 基于 SQLite 的证券目录快照存储。
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 打开或创建证券目录 SQLite 数据库。
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create symbol database directory: %w", err)
	}
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)", path, sqliteBusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open symbol database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize symbol database: %w", err)
	}
	// 旧库补 kind 列；已存在则忽略
	if _, err := db.Exec(`ALTER TABLE symbol_directory ADD COLUMN kind TEXT NOT NULL DEFAULT 'stock'`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			_ = db.Close()
			return nil, fmt.Errorf("migrate symbol directory kind column: %w", err)
		}
	}
	return &SQLiteStore{db: db}, nil
}

// Load 读取证券目录快照；无快照时返回 found=false。
func (s *SQLiteStore) Load() (Snapshot, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("begin symbol directory read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var loadedAtUnix int64
	err = tx.QueryRow(`SELECT loaded_at_unix FROM symbol_directory_metadata WHERE id = 1`).Scan(&loadedAtUnix)
	if err == sql.ErrNoRows {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("read symbol directory metadata: %w", err)
	}

	rows, err := tx.Query(`
		SELECT source, param_kind, param_value, symbol, description, exchange, kind
		FROM symbol_directory
		ORDER BY rowid`)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("read symbol directory: %w", err)
	}
	entries := make([]Item, 0)
	for rows.Next() {
		var item Item
		var paramKind string
		var paramValue uint8
		var kind string
		if err := rows.Scan(&item.Source, &paramKind, &paramValue, &item.Symbol, &item.Description, &item.Exchange, &kind); err != nil {
			_ = rows.Close()
			return Snapshot{}, false, fmt.Errorf("scan symbol directory: %w", err)
		}
		if kind == "" {
			kind = KindStock
		}
		item.Params = map[string]any{paramKind: paramValue, "kind": kind}
		entries = append(entries, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Snapshot{}, false, fmt.Errorf("iterate symbol directory: %w", err)
	}
	if err := rows.Close(); err != nil {
		return Snapshot{}, false, fmt.Errorf("close symbol directory rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, false, fmt.Errorf("commit symbol directory read: %w", err)
	}
	return Snapshot{
		Entries:  entries,
		LoadedAt: time.Unix(loadedAtUnix, 0).UTC(),
	}, true, nil
}

// Replace 以事务方式替换证券目录快照，失败时回滚保持原快照。
func (s *SQLiteStore) Replace(snapshot Snapshot) error {
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
			(source, param_kind, param_value, symbol, description, exchange, kind)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare symbol directory insert: %w", err)
	}
	defer stmt.Close()

	for _, item := range snapshot.Entries {
		paramKind, paramValue, kind, err := directoryParam(item.Params)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(item.Source, paramKind, paramValue, item.Symbol, item.Description, item.Exchange, kind); err != nil {
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

// directoryParam 拆出 SQLite 主键用的 market|category，以及品种 kind
func directoryParam(params map[string]any) (paramKind string, paramValue uint8, kind string, err error) {
	if params == nil {
		return "", 0, "", fmt.Errorf("symbol directory params are empty")
	}
	kind = KindStock
	if k, ok := params["kind"].(string); ok && k != "" {
		kind = k
	}
	for key, value := range params {
		if key == "kind" {
			continue
		}
		if key != "market" && key != "category" {
			return "", 0, "", fmt.Errorf("unsupported symbol directory param %q", key)
		}
		v, ok := anyToUint8(value)
		if !ok {
			return "", 0, "", fmt.Errorf("symbol directory param %q must be uint8", key)
		}
		if paramKind != "" {
			return "", 0, "", fmt.Errorf("symbol directory params must contain exactly one market/category entry")
		}
		paramKind, paramValue = key, v
	}
	if paramKind == "" {
		return "", 0, "", fmt.Errorf("symbol directory params must contain market or category")
	}
	return paramKind, paramValue, kind, nil
}

func anyToUint8(v any) (uint8, bool) {
	switch n := v.(type) {
	case uint8:
		return n, true
	case int:
		if n < 0 || n > 255 {
			return 0, false
		}
		return uint8(n), true
	case int64:
		if n < 0 || n > 255 {
			return 0, false
		}
		return uint8(n), true
	case float64:
		if n < 0 || n > 255 || n != float64(uint8(n)) {
			return 0, false
		}
		return uint8(n), true
	default:
		return 0, false
	}
}

// Close 关闭底层数据库连接。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
