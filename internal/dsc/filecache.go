package dsc

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/outfmt"
)

// fileCacheEntry 是 files.json 中的一条记录：本地文件 -> 远端 file_id。
type fileCacheEntry struct {
	FileID     string `json:"file_id"`
	Size       int64  `json:"size"`
	Purpose    string `json:"purpose"`
	UploadedAt int64  `json:"uploaded_at"`
	// ExpiresAt 远端文件的到期时间戳（0 表示永久）。
	// 命中缓存前会检查过期，避免返回已失效的 file_id。
	ExpiresAt int64 `json:"expires_at,omitzero"`
}

// fileCache 是 files.json 的完整结构。
// Version 字段预留：未来格式变化时可用于迁移/重写缓存。
type fileCache struct {
	Version int                       `json:"version"`
	Files   map[string]fileCacheEntry `json:"files"`
}

const fileCacheVersion = 1

// fileCacheKey 计算缓存 key：<size>\0<sha256hex>。
// size 参与 key 与 git blob 格式（<type> <size>\0<content>）精神一致：
// SHA-256 碰撞概率已可忽略，但带上 size 可彻底排除"同 hash 异内容"
// 的边界情况（内容本身不存入 key，避免 files.json 随文件体积膨胀）。
func fileCacheKey(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d\x00%x", size, h.Sum(nil)), nil
}

// defaultCachePath 返回本地文件缓存的默认路径（~/.dscli/files.json）。
func defaultCachePath() string {
	return filepath.Join(config.ConfigDir, "files.json")
}

// loadFileCache 读取 files.json。文件不存在或损坏时返回空缓存——
// 本地缓存只是加速手段，绝不能因此阻塞上传。
func loadFileCache(path string) *fileCache {
	c := &fileCache{Version: fileCacheVersion, Files: make(map[string]fileCacheEntry)}
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			outfmt.Debug("files cache 读取失败（忽略）: %v\n", err)
		}
		return c
	}
	if err := json.Unmarshal(data, c); err != nil {
		outfmt.Debug("files cache 解析失败（忽略，将重新上传）: %v\n", err)
		return &fileCache{Version: fileCacheVersion, Files: make(map[string]fileCacheEntry)}
	}
	if c.Files == nil {
		c.Files = make(map[string]fileCacheEntry)
	}
	return c
}

// saveFileCache 原子写入 files.json（唯一临时文件 + rename）。
// 并发写时不会交错写入同一路径（唯一名避免损坏），rename 保证最终
// 文件要么是旧完整版要么是新完整版。并发 read-modify-write 的丢更新
// 后果仅是缓存 miss（下次重传），可接受——缓存只是加速手段。
func saveFileCache(path string, c *fileCache) error {
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".files.json-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 成功 rename 后无效果
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// cacheLookup 在缓存中查找 key；命中且 purpose/有效期一致时构造
// FileObject 直接返回（created_at 用上传时间近似，调用方主要使用 ID）。
// expiresSeconds 是新请求期望的有效期秒数（0 表示永久）：命中要求
// "永久/限时"类别一致；限时文件已过期视为 miss。同类别下具体秒数
// 差异容忍（复用旧文件生命周期），避免每次上传都因秒数不同而重传。
func (f *FilesAPI) cacheLookup(key, purpose, baseName string, expiresSeconds int64) (*FileObject, bool) {
	if key == "" || f.cachePath == "" {
		return nil, false
	}
	c := loadFileCache(f.cachePath)
	e, ok := c.Files[key]
	if !ok || e.FileID == "" {
		return nil, false
	}
	if purpose != "" && e.Purpose != "" && e.Purpose != purpose {
		// purpose 不匹配：视为 miss，重新上传并覆盖缓存。
		return nil, false
	}
	if e.ExpiresAt != 0 && time.Now().Unix() > e.ExpiresAt {
		// 缓存文件已过期：miss，重新上传。
		return nil, false
	}
	if (expiresSeconds == 0) != (e.ExpiresAt == 0) {
		// 永久/限时类别不一致：miss（例如之前限时上传，现在要永久）。
		return nil, false
	}
	return &FileObject{
		ID:        e.FileID,
		Object:    "file",
		Bytes:     e.Size,
		CreatedAt: e.UploadedAt,
		Filename:  baseName,
		Purpose:   purpose,
		ExpiresAt: e.ExpiresAt,
	}, true
}

// cacheStore 写入一条缓存记录；失败仅 Debug 提示，不影响上传结果。
func (f *FilesAPI) cacheStore(key string, file *FileObject) {
	if key == "" || f.cachePath == "" {
		return
	}
	c := loadFileCache(f.cachePath)
	c.Files[key] = fileCacheEntry{
		FileID:     file.ID,
		Size:       file.Bytes,
		Purpose:    file.Purpose,
		UploadedAt: file.CreatedAt,
		ExpiresAt:  file.ExpiresAt,
	}
	if err := saveFileCache(f.cachePath, c); err != nil {
		outfmt.Debug("files cache 写入失败（忽略）: %v\n", err)
	}
}

// cacheRemoveByFileID 从缓存中删除指定 file_id 的记录（删除远端文件后
// 必须清理，否则 Upload 会命中已失效的 file_id）。
func (f *FilesAPI) cacheRemoveByFileID(fileID string) {
	if fileID == "" || f.cachePath == "" {
		return
	}
	c := loadFileCache(f.cachePath)
	removed := false
	for k, e := range c.Files {
		if e.FileID == fileID {
			delete(c.Files, k)
			removed = true
		}
	}
	if removed {
		if err := saveFileCache(f.cachePath, c); err != nil {
			outfmt.Debug("files cache 删除记录失败（忽略）: %v\n", err)
		}
	}
}
