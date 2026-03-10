package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	// ModelID = int64(0)
	dbPath = func() string {
		path := filepath.Join(ConfigDir, "sqlite.db")
		insideShellExec := os.Getenv("InsideShellExec")
		if insideShellExec != "" {
			dir := filepath.Join(ConfigDir, "inside-shell-exec")
			err := os.MkdirAll(dir, 0o755)
			if err != nil {
				panic(err)
			}
			name := filepath.Base(os.Args[0])
			pid := os.Getpid()
			path = filepath.Join(dir, fmt.Sprintf("%s-%d.db", name, pid))
		}
		return path
	}()

	// 注册队列
	tableSchemas   = []string{}
	indexSchemas   = []string{}
	upgradeSchemas = []string{}
	postInitHooks  = []func(*sql.DB) error{}

	// 数据库连接
	dbOnce sync.Once
	dbErr  error
)

func RegisterTableSchema(ss ...string) {
	tableSchemas = append(tableSchemas, ss...)
}

func RegisterIndexSchema(ss ...string) {
	indexSchemas = append(indexSchemas, ss...)
}

func RegisterUpgradeSchema(ss ...string) {
	upgradeSchemas = append(upgradeSchemas, ss...)
}

func RegisterPostInitHook(hook func(*sql.DB) error) {
	postInitHooks = append(postInitHooks, hook)
}

// 初始化数据库（延迟执行，确保所有init()已完成）
func initDatabase(db *sql.DB) error {
	// 1. 创建表
	for _, query := range tableSchemas {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("创建表失败: %w\nSQL: %s", err, query[:100])
		}
	}

	// 2. 创建索引
	for _, query := range indexSchemas {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("创建索引失败: %w\nSQL: %s", err, query[:100])
		}
	}

	// 3. 执行升级脚本
	for _, query := range upgradeSchemas {
		if _, err := db.Exec(query); err == nil {
			Debug("升级完成: %s\n", query[:50])
		}
	}

	// 4. 执行后初始化钩子
	for _, hook := range postInitHooks {
		if err := hook(db); err != nil {
			Debug("后初始化钩子失败: %v\n", err)
		}
	}

	return nil
}

// OpenDB 打开数据库连接（确保数据库已初始化）
func OpenDB(elem ...string) (*sql.DB, error) {
	var err error
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	dbOnce.Do(func() {
		dbErr = initDatabase(db)
	})

	if dbErr != nil {
		return nil, dbErr
	}

	// 如果指定了其他数据库路径
	if len(elem) > 0 {
		dbPath := filepath.Join(elem...)
		return sql.Open("sqlite", dbPath)
	}

	return db, nil
}

// SetDBPath 设置数据库文件路径
func SetDBPath(path string) {
	if path != "" {
		dbPath = path
	}
}

func GetDBPath() string {
	return dbPath
}
