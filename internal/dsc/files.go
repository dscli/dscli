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
type FilesAPI struct {
	apiKey  string
	baseURL string
}

// NewFilesAPI 创建 Files API 客户端。
func NewFilesAPI(apiKey, baseURL string) *FilesAPI {
	return &FilesAPI{apiKey: apiKey, baseURL: baseURL}
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
}

// Upload 上传本地图片文件，返回文件对象（含 file_id）。
// 文件流式读取，不整体载入内存；最大 64 MiB。
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

// Delete 删除文件。
func (f *FilesAPI) Delete(ctx context.Context, fileID string) (*FileDeleteResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "FilesAPI.Delete")
	defer span.Finish()

	var result FileDeleteResult
	if err := f.doJSON(ctx, http.MethodDelete, "/files/"+url.PathEscape(fileID), nil, &result); err != nil {
		return nil, err
	}
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
