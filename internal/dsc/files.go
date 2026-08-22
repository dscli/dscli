package dsc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/nanjj/clog"
)

// FileObject 是 Files API 返回的文件对象（OpenAI 兼容格式）。
type FileObject struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int64  `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
	ExpiresAt int64  `json:"expires_at,omitzero"`
}

// FileList 是 Files API 的分页列表响应。
type FileList struct {
	Object  string       `json:"object"`
	Data    []FileObject `json:"data"`
	FirstID string       `json:"first_id"`
	LastID  string       `json:"last_id"`
	HasMore bool         `json:"has_more"`
}

// FileDeleteResult 是删除文件的响应。
type FileDeleteResult struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Deleted bool   `json:"deleted"`
}

// FilesAPI 封装 DeepSeek Files API（OpenAI 兼容端点）。
// 与 Chat 使用同一 base URL 与 API key。
// Upload 内置本地内容缓存（~/.dscli/files.json）：相同内容（SHA-256+大小）
// 的文件重复上传直接复用已上传的 file_id，不产生网络请求。
type FilesAPI struct {
	apiKey    string
	baseURL   string
	cachePath string // 本地缓存文件路径；空字符串表示禁用缓存
}

// NewFilesAPI 创建 Files API 客户端（使用默认缓存路径 ~/.dscli/files.json）。
func NewFilesAPI(apiKey, baseURL string) *FilesAPI {
	return &FilesAPI{apiKey: apiKey, baseURL: baseURL, cachePath: defaultCachePath()}
}

// NewFilesAPIWithCache 创建 Files API 客户端，使用指定缓存路径。
// cachePath 为空字符串可禁用缓存（主要用于测试隔离）。
func NewFilesAPIWithCache(apiKey, baseURL, cachePath string) *FilesAPI {
	return &FilesAPI{apiKey: apiKey, baseURL: baseURL, cachePath: cachePath}
}

// WithCachePath 覆盖缓存文件路径并返回自身（链式调用）；
// 传入空字符串禁用缓存。
func (f *FilesAPI) WithCachePath(path string) *FilesAPI {
	f.cachePath = path
	return f
}

// MaxFileBytes 是单个上传文件的最大大小（64 MiB，见 Files API 限制）。
const MaxFileBytes = 64 * 1024 * 1024

// UploadOptions 是上传文件的可选参数。
type UploadOptions struct {
	// Purpose 用途，固定 user_data（空时使用默认值）。
	Purpose string
	// ExpiresSeconds 有效期秒数，取值 3600-2592000（1 小时到 30 天）；
	// 0 表示永久有效（不传 expires_after 字段）。
	ExpiresSeconds int64
	// NoCache 跳过本地缓存强制重新上传（上传结果也不写入缓存）。
	// 用于文件在远端被删除后重建，或测试缓存逻辑。
	NoCache bool
}

// Upload 上传本地图片文件，返回文件对象（含 file_id）。
// 文件流式读取，不整体载入内存；最大 64 MiB。
// 相同内容（SHA-256 + 大小，见 fileCacheKey）的文件如果已在本地缓存中，
// 直接返回上次的 file_id 而不发起网络请求。
func (f *FilesAPI) Upload(ctx context.Context, path string, opts UploadOptions) (*FileObject, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "FilesAPI.Upload")
	defer span.Finish()

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("文件不存在: %w", err)
	}
	if info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("文件 %s 大小 %d 字节，超过上限 64 MiB", path, info.Size())
	}
	purpose := opts.Purpose
	if purpose == "" {
		purpose = "user_data"
	}

	// 缓存查找：命中直接返回，零网络请求。
	var cacheKey string
	if !opts.NoCache && f.cachePath != "" {
		key, hashErr := fileCacheKey(path, info.Size())
		if hashErr != nil {
			outfmt.Debug("files cache 哈希失败（忽略）: %v\n", hashErr)
		} else {
			cacheKey = key
			if file, ok := f.cacheLookup(cacheKey, purpose, filepath.Base(path), opts.ExpiresSeconds); ok {
				outfmt.Debug("files cache hit: %s -> %s\n", path, file.ID)
				return file, nil
			}
		}
	}

	// 流式 multipart 编码：文件内容通过 pipe 直接送给 HTTP 客户端，
	// 避免把大图读入内存。
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		fw, err := mw.CreateFormFile("file", filepath.Base(path))
		if err != nil {
			return
		}
		fh, err := os.Open(path)
		if err != nil {
			return
		}
		defer fh.Close()
		if _, err := io.Copy(fw, fh); err != nil {
			return
		}
		_ = mw.WriteField("purpose", purpose)
		if opts.ExpiresSeconds > 0 {
			_ = mw.WriteField("expires_after[anchor]", "created_at")
			_ = mw.WriteField("expires_after[seconds]", strconv.FormatInt(opts.ExpiresSeconds, 10))
		}
		_ = mw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.baseURL+"/files", pr)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	defer outfmt.DebugBytes("", respBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiError(resp.StatusCode, respBody)
	}
	var file FileObject
	if err := json.Unmarshal(respBody, &file); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	// 上传成功：写缓存前复核文件内容未在上传期间变化（TOCTOU），
	// 否则缓存 key 可能与实际上传内容错配（先 hash 后上传是两次读取）。
	if cacheKey != "" {
		if info2, statErr := os.Stat(path); statErr == nil {
			if key2, hashErr := fileCacheKey(path, info2.Size()); hashErr == nil && key2 == cacheKey {
				f.cacheStore(cacheKey, &file)
			} else {
				outfmt.Debug("files cache: 上传期间文件已变化，跳过缓存: %s\n", path)
			}
		}
	}
	return &file, nil
}

// List 列出已上传的文件，可按分页/排序/用途过滤。
func (f *FilesAPI) List(ctx context.Context, after string, limit int, order, purpose string) (*FileList, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "FilesAPI.List")
	defer span.Finish()

	q := url.Values{}
	if after != "" {
		q.Set("after", after)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if order != "" {
		q.Set("order", order)
	}
	if purpose != "" {
		q.Set("purpose", purpose)
	}
	var list FileList
	if err := f.doJSON(ctx, http.MethodGet, "/files", q, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// Info 查询单个文件信息。
func (f *FilesAPI) Info(ctx context.Context, fileID string) (*FileObject, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "FilesAPI.Info")
	defer span.Finish()

	var file FileObject
	if err := f.doJSON(ctx, http.MethodGet, "/files/"+url.PathEscape(fileID), nil, &file); err != nil {
		return nil, err
	}
	return &file, nil
}

// Delete 删除文件，同时清理对应的本地缓存记录（避免后续 Upload
// 命中已被删除的 file_id）。
func (f *FilesAPI) Delete(ctx context.Context, fileID string) (*FileDeleteResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "FilesAPI.Delete")
	defer span.Finish()

	var result FileDeleteResult
	if err := f.doJSON(ctx, http.MethodDelete, "/files/"+url.PathEscape(fileID), nil, &result); err != nil {
		return nil, err
	}
	f.cacheRemoveByFileID(result.ID)
	return &result, nil
}

// doJSON 发送无 body 的 JSON 请求（GET/DELETE）。
func (f *FilesAPI) doJSON(ctx context.Context, method, path string, query url.Values, result any) error {
	fullURL := f.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	defer outfmt.DebugBytes("", respBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError(resp.StatusCode, respBody)
	}
	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}
	return nil
}

// apiError 解析 Files API 错误响应体，尽量给出错误消息。
func apiError(status int, body []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Error.Message != "" {
		return fmt.Errorf("API 错误 %d: %s (%s)", status, e.Error.Message, e.Error.Type)
	}
	return fmt.Errorf("API 返回错误状态码 %d: %s", status, string(body))
}
