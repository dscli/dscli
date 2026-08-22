// Package vision 提供 DeepSeek Files API（图片上传/管理）工具。
// 上传的文件通过 file_id 在图像理解请求中引用（file 内容块）。
package vision

import (
	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/dsc"
)

// newFilesAPI 从配置构造 Files API 客户端（与 chat 使用同一 key/baseURL）。
func newFilesAPI() *dsc.FilesAPI {
	key := config.Get("deepseek-api-key", "")
	url := config.Get("deepseek-base-url", "https://api.deepseek.com")
	return dsc.NewFilesAPI(key, url)
}
