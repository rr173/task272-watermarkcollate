package store

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 连接与幂等迁移。
// 单连接 + 互斥锁串行化写入，规避 SQLite 单写者限制；WAL 与 busy_timeout 保证重启后一致恢复。
// 列表查询（ListLeaves 等）一律直读数据库，不复用任何内存缓存：
// 纸页状态一旦落库即须对折页连续性校验等下游即时可见，缓存会掩盖待解析→有效等迁移，
// 也会在并发写入时引发 map 竞态，故此处不持有任何列表快照缓存。
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// Open 打开（必要时创建）数据库文件并执行幂等迁移。
// dsn 形如 "file:data.db?cache=shared&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"。
func Open(path string) (*Store, error) {
	if path != "" && path != ":memory:" {
		if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?cache=shared&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

// Close 关闭底层连接。
func (s *Store) Close() error { return s.db.Close() }

// Now 统一取当前时间（便于测试固定时钟）。
var Now = func() time.Time { return time.Now() }

// migrate 幂等建表：所有约束（唯一键、CHECK）在数据库层落地，应用层校验只是前置防御。
func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS manuscripts (
	id          TEXT PRIMARY KEY,
	title       TEXT NOT NULL,
	period      TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL,
	version     INTEGER NOT NULL DEFAULT 1,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS leaves (
	id           TEXT PRIMARY KEY,
	manuscript_id TEXT NOT NULL REFERENCES manuscripts(id),
	page_no      INTEGER NOT NULL,
	quire_no     INTEGER NOT NULL,
	position     TEXT NOT NULL,
	status       TEXT NOT NULL,
	binding_edge TEXT NOT NULL,
	chain_deg    INTEGER NOT NULL,
	width_mm     REAL NOT NULL,
	height_mm    REAL NOT NULL,
	confidence   REAL NOT NULL,
	notes        TEXT NOT NULL DEFAULT '',
	version      INTEGER NOT NULL DEFAULT 1,
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL,
	UNIQUE(manuscript_id, page_no)
);
CREATE INDEX IF NOT EXISTS idx_leaves_manuscript ON leaves(manuscript_id, quire_no, page_no);

CREATE TABLE IF NOT EXISTS watermark_observations (
	id          TEXT PRIMARY KEY,
	leaf_id     TEXT NOT NULL REFERENCES leaves(id),
	half_id     TEXT NOT NULL,
	mold_pair_id TEXT NOT NULL,
	position    TEXT NOT NULL,
	x_mm        REAL NOT NULL,
	y_mm        REAL NOT NULL,
	rotation_deg REAL NOT NULL,
	confidence  REAL NOT NULL,
	status      TEXT NOT NULL,
	notes       TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL,
	UNIQUE(leaf_id, half_id)
);
CREATE INDEX IF NOT EXISTS idx_watermarks_mold ON watermark_observations(mold_pair_id, position);

CREATE TABLE IF NOT EXISTS watermark_pairings (
	id             TEXT PRIMARY KEY,
	manuscript_id  TEXT NOT NULL REFERENCES manuscripts(id),
	watermark_a_id TEXT NOT NULL REFERENCES watermark_observations(id),
	watermark_b_id TEXT NOT NULL REFERENCES watermark_observations(id),
	mold_pair_id   TEXT NOT NULL,
	score          REAL NOT NULL,
	status         TEXT NOT NULL,
	evidence       TEXT NOT NULL DEFAULT '',
	version        INTEGER NOT NULL DEFAULT 1,
	created_at     TEXT NOT NULL,
	confirmed_at   TEXT,
	UNIQUE(watermark_a_id, watermark_b_id)
);
CREATE INDEX IF NOT EXISTS idx_pairings_manuscript ON watermark_pairings(manuscript_id, status);

CREATE TABLE IF NOT EXISTS leaf_relations (
	id               TEXT PRIMARY KEY,
	manuscript_id    TEXT NOT NULL REFERENCES manuscripts(id),
	left_leaf_id     TEXT NOT NULL REFERENCES leaves(id),
	right_leaf_id    TEXT NOT NULL REFERENCES leaves(id),
	page_delta       INTEGER NOT NULL,
	chain_consistent INTEGER NOT NULL DEFAULT 0,
	watermark_score  REAL NOT NULL DEFAULT 0,
	fold_continuous  INTEGER NOT NULL DEFAULT 0,
	gap_reasons      TEXT NOT NULL DEFAULT '',
	verdict          TEXT NOT NULL,
	evidence         TEXT NOT NULL DEFAULT '',
	adjudicator      TEXT NOT NULL DEFAULT '',
	version          INTEGER NOT NULL DEFAULT 1,
	created_at       TEXT NOT NULL,
	adjudicated_at   TEXT,
	UNIQUE(left_leaf_id, right_leaf_id)
);
CREATE INDEX IF NOT EXISTS idx_relations_manuscript ON leaf_relations(manuscript_id, verdict);

CREATE TABLE IF NOT EXISTS collation_versions (
	id            TEXT PRIMARY KEY,
	manuscript_id TEXT NOT NULL REFERENCES manuscripts(id),
	version_no    INTEGER NOT NULL,
	status        TEXT NOT NULL,
	summary       TEXT NOT NULL DEFAULT '',
	content_json  TEXT NOT NULL DEFAULT '',
	created_at    TEXT NOT NULL,
	frozen_at     TEXT,
	superseded_at TEXT,
	UNIQUE(manuscript_id, version_no)
);
CREATE INDEX IF NOT EXISTS idx_versions_manuscript ON collation_versions(manuscript_id, status);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("迁移建表: %w", err)
	}
	return nil
}

// WithTx 在互斥锁保护的事务中执行 fn。
// 锁覆盖整个 Begin/fn/Commit，避免 SQLite 单写者与乐观锁检查窗口被并发切开；
// defer Rollback 保证 fn 失败或 panic 时事务被释放，Commit 成功后 Rollback 成为 no-op。
func (s *Store) WithTx(fn func(tx *sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务: %w", err)
	}
	return nil
}
