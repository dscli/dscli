package wechat

import (
	"database/sql"
	"io"
	"sync"
	"time"

	"gitcode.com/dscli/dscli/internal/sqlite"
)

// SQLiteHotReloadStorage 实现 openwechat.HotReloadStorage 接口
type SQLiteHotReloadStorage struct {
	dbPath  string
	account string
	db      *sql.DB
	lock    sync.Mutex
}

// NewSQLiteHotReloadStorage 创建SQLite热存储
func NewSQLiteHotReloadStorage(dbPath, account string) (*SQLiteHotReloadStorage, error) {
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, err
	}

	// 初始化数据库表
	if err := initDatabase(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteHotReloadStorage{
		dbPath:  dbPath,
		account: account,
		db:      db,
	}, nil
}

// Read 实现 io.Reader 接口
func (s *SQLiteHotReloadStorage) Read(p []byte) (n int, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// 从数据库读取会话数据
	var sessionData []byte
	err = s.db.QueryRow(`
		SELECT session_data FROM wechat_sessions 
		WHERE account = ? AND is_active = 1 
		ORDER BY updated_at DESC LIMIT 1
	`, s.account).Scan(&sessionData)

	if err == sql.ErrNoRows {
		return 0, io.EOF // 没有会话数据
	}
	if err != nil {
		return 0, err
	}

	// 复制数据到p
	n = copy(p, sessionData)
	if n < len(sessionData) {
		// 数据太大，p装不下
		return n, io.ErrShortBuffer
	}

	return n, nil
}

// Write 实现 io.Writer 接口
func (s *SQLiteHotReloadStorage) Write(p []byte) (n int, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// 保存或更新会话数据
	_, err = s.db.Exec(`
		INSERT INTO wechat_sessions (account, session_data, updated_at, is_active)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(account) DO UPDATE SET
			session_data = excluded.session_data,
			updated_at = excluded.updated_at,
			is_active = 1
	`, s.account, p, time.Now())
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// Close 实现 io.Closer 接口
func (s *SQLiteHotReloadStorage) Close() error {
	return s.db.Close()
}

// initDatabase 初始化数据库表
func initDatabase(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS wechat_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account TEXT NOT NULL,
			session_data BLOB NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			is_active BOOLEAN DEFAULT 1,
			UNIQUE(account)
		);
		
		CREATE TABLE IF NOT EXISTS wechat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER REFERENCES wechat_sessions(id),
			wx_msg_id TEXT,
			direction TEXT,
			from_user TEXT,
			to_user TEXT,
			content TEXT,
			msg_type TEXT,
			status TEXT DEFAULT 'unread',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			replied_at DATETIME,
			reply_content TEXT
		);
		
		CREATE INDEX IF NOT EXISTS idx_sessions_account ON wechat_sessions(account);
		CREATE INDEX IF NOT EXISTS idx_sessions_active ON wechat_sessions(is_active);
		CREATE INDEX IF NOT EXISTS idx_messages_session ON wechat_messages(session_id);
		CREATE INDEX IF NOT EXISTS idx_messages_status ON wechat_messages(status);
	`)
	return err
}
