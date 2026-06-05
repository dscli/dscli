package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/lockfile"
	"github.com/dscli/dscli/internal/outfmt"
)

var (
	dbPath = func() string {
		configDir := config.ConfigDir
		isTesting := context.IsTesting()

		// 测试环境：使用临时目录数据库，避免污染生产数据
		if isTesting {
			name := filepath.Base(os.Args[0])
			return filepath.Join(os.TempDir(),
				fmt.Sprintf("dscli-test-%s-%d.db", name, os.Getpid()))
		}

		// 生产环境
		return filepath.Join(configDir, "sqlite.db")
	}()

	// 注册队列
	tableSchemas   = []string{}
	indexSchemas   = []string{}
	upgradeSchemas = []string{}
	postInitHooks  = []func(*DB) error{}

	// 数据库连接
	dbOnce sync.Once
	dbErr  error
)

// DB 包装 *sql.DB，在 Close 时释放文件锁。
//
// 嵌入 *sql.DB 使得所有 database/sql 方法自动提升，
// 现有调用方无需改动即可继续使用 db.Exec / db.Query 等。
type DB struct {
	*sql.DB
	lk *lockfile.Lock
}

// Close 关闭数据库连接并释放文件锁（如有）。
func (db *DB) Close() error {
	var err error
	if db.DB != nil {
		err = db.DB.Close()
	}
	if db.lk != nil {
		if lkErr := db.lk.Close(); lkErr != nil && err == nil {
			err = lkErr
		}
	}
	return err
}

func RegisterTableSchema(ss ...string) {
	tableSchemas = append(tableSchemas, ss...)
}

func RegisterIndexSchema(ss ...string) {
	indexSchemas = append(indexSchemas, ss...)
}

func RegisterUpgradeSchema(ss ...string) {
	upgradeSchemas = append(upgradeSchemas, ss...)
}

func RegisterPostInitHook(hook func(*DB) error) {
	postInitHooks = append(postInitHooks, hook)
}

// 初始化数据库（延迟执行，确保所有init()已完成）
func initDatabase(db *DB) error {
	// 0. 确保 db_metadata 表存在（用于检测 DB 重建）
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS db_metadata (
		key TEXT PRIMARY KEY,
		value TEXT
	)`); err != nil {
		return fmt.Errorf("创建 db_metadata 表失败: %w", err)
	}

	// 读取上次成功初始化的版本
	var storedVersion string
	readErr := db.QueryRow(`SELECT value FROM db_metadata WHERE key = 'version'`).Scan(&storedVersion)
	currentVersion := config.BuildTime

	// 版本比对：同版本跳过，否则跑全量初始化（所有 DDL 均为 IF NOT EXISTS）
	if readErr == nil && currentVersion != "" && storedVersion == currentVersion {
		return nil
	}

	// 1. 创建表
	for _, query := range tableSchemas {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("创建表失败: %w\nSQL: %s", err, query)
		}
	}

	// 2. 创建索引
	for _, query := range indexSchemas {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("创建索引失败: %w\nSQL: %s", err, query)
		}
	}

	// 3. 执行升级脚本
	for _, query := range upgradeSchemas {
		if _, err := db.Exec(query); err == nil {
			outfmt.Debug("升级完成: %s\n", query)
		}
	}

	// 4. 执行后初始化钩子
	for _, hook := range postInitHooks {
		if err := hook(db); err != nil {
			outfmt.Debug("后初始化钩子失败: %v\n", err)
		}
	}

	// 全部完成后记录当前版本（仅当 BuildTime 已注入）
	if currentVersion != "" {
		if _, err := db.Exec(`INSERT OR REPLACE INTO db_metadata (key, value) VALUES ('version', ?)`, currentVersion); err != nil {
			outfmt.Debug("记录版本失败: %v\n", err)
		}
	}

	return nil
}

// OpenDB 打开数据库连接（确保数据库已初始化）。
//
// 对于默认生产数据库（~/.dscli/sqlite.db），自动获取文件锁以防止
// 多进程并发导致的 SQLITE_BUSY。测试环境和自定义路径不获取锁。
func OpenDB(elem ...string) (*DB, error) {
	// 自定义数据库路径（elem 指定）— 不获取锁
	if len(elem) > 0 {
		rawDB, err := Open(filepath.Join(elem...))
		if err != nil {
			return nil, err
		}
		return &DB{DB: rawDB}, nil
	}

	// 仅在默认生产路径获取文件锁
	defaultPath := filepath.Join(config.ConfigDir, "sqlite.db")
	if dbPath == defaultPath {
		lk, err := lockfile.LockDB("sqlite.db")
		if err != nil {
			return nil, fmt.Errorf("获取数据库锁失败: %w", err)
		}

		rawDB, err := Open(dbPath)
		if err != nil {
			lk.Close()
			return nil, fmt.Errorf("打开数据库失败: %w", err)
		}

		db := &DB{DB: rawDB, lk: lk}

		dbOnce.Do(func() {
			dbErr = initDatabase(db)
		})

		if dbErr != nil {
			db.Close()
			return nil, dbErr
		}

		return db, nil
	}

	// 非默认路径（测试环境 / SetDBPath 自定义）— 不获取锁
	rawDB, err := Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	db := &DB{DB: rawDB}

	dbOnce.Do(func() {
		dbErr = initDatabase(db)
	})

	if dbErr != nil {
		db.Close()
		return nil, dbErr
	}

	return db, nil
}

// SetDBPath 设置数据库文件路径
func SetDBPath(path string) {
	if path != "" {
		dbPath = path
		dbOnce = sync.Once{}
	}
}

func GetDBPath() string {
	return dbPath
}

// GetMetadata 读取 db_metadata 表中的值，用于诊断 DB 状态。
// 返回空字符串表示 key 不存在（DB 尚未初始化完成）。
func GetMetadata(key string) string {
	db, err := OpenDB()
	if err != nil {
		return ""
	}
	defer db.Close()

	var value string
	_ = db.QueryRow(`SELECT value FROM db_metadata WHERE key = ?`, key).Scan(&value)
	return value
}
