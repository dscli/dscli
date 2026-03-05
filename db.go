package main

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	HistoryLimit = &struct{}{}
	ModelID      = int64(0)
	DBPath       = filepath.Join(ConfigDir, "sqlite.db")

	// 注册队列
	tableSchemas   = []string{}
	indexSchemas   = []string{}
	upgradeSchemas = []string{}
	postInitHooks  = []func(*sql.DB) error{}

	// 数据库连接
	dbOnce sync.Once
	db     *sql.DB
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
func initDatabase() error {
	fmt.Println("初始化数据库...")

	var err error
	db, err = sql.Open("sqlite", DBPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}

	// 1. 创建表
	fmt.Printf("创建 %d 个表\n", len(tableSchemas))
	for _, query := range tableSchemas {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("创建表失败: %w\nSQL: %s", err, query[:100])
		}
	}

	// 2. 创建索引
	fmt.Printf("创建 %d 个索引\n", len(indexSchemas))
	for _, query := range indexSchemas {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("创建索引失败: %w\nSQL: %s", err, query[:100])
		}
	}

	// 3. 执行升级脚本
	fmt.Printf("执行 %d 个升级脚本\n", len(upgradeSchemas))
	for _, query := range upgradeSchemas {
		if _, err := db.Exec(query); err == nil {
			fmt.Printf("升级完成: %s\n", query[:50])
		}
	}

	// 4. 执行后初始化钩子
	fmt.Printf("执行 %d 个后初始化钩子\n", len(postInitHooks))
	for _, hook := range postInitHooks {
		if err := hook(db); err != nil {
			fmt.Printf("后初始化钩子失败: %v\n", err)
		}
	}

	fmt.Println("数据库初始化完成")
	return nil
}

// OpenDB 打开数据库连接（确保数据库已初始化）
func OpenDB(elem ...string) (*sql.DB, error) {
	dbOnce.Do(func() {
		dbErr = initDatabase()
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
		DBPath = path
	}
}
