package wechat

import "fmt"

// 定义清晰的错误类型
var (
	// 登录相关错误
	ErrNotLoggedIn     = fmt.Errorf("微信未登录")
	ErrSessionExpired  = fmt.Errorf("微信会话已过期")
	ErrLoginFailed     = fmt.Errorf("微信登录失败")
	ErrNeedScanQRCode  = fmt.Errorf("需要扫码登录")
	ErrNeedPushConfirm = fmt.Errorf("需要在手机上确认登录")

	// 消息相关错误
	ErrMessageNotFound   = fmt.Errorf("消息不存在")
	ErrMessageTooLong    = fmt.Errorf("消息过长")
	ErrMessageSendFailed = fmt.Errorf("消息发送失败")
	ErrNoUnreadMessages  = fmt.Errorf("没有未读消息")

	// 联系人相关错误
	ErrContactNotFound  = fmt.Errorf("联系人不存在")
	ErrInvalidContactID = fmt.Errorf("无效的联系人ID")

	// 网络相关错误
	ErrNetworkError      = fmt.Errorf("网络连接失败")
	ErrWechatServerError = fmt.Errorf("微信服务器错误")
	ErrRateLimited       = fmt.Errorf("操作频率过高，请稍后重试")

	// 配置相关错误
	ErrConfigNotFound = fmt.Errorf("配置文件不存在")
	ErrInvalidConfig  = fmt.Errorf("无效的配置")
	ErrStorageError   = fmt.Errorf("存储错误")

	// 权限相关错误
	ErrPermissionDenied    = fmt.Errorf("权限不足")
	ErrOperationNotAllowed = fmt.Errorf("操作不被允许")
)

// wrapErr 包装错误，提供上下文信息
func wrapErr(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// IsRetryableError 判断错误是否可重试
func IsRetryableError(err error) bool {
	switch err {
	case ErrNetworkError, ErrWechatServerError, ErrRateLimited:
		return true
	default:
		return false
	}
}

// ShouldReLoginError 判断错误是否需要重新登录
func ShouldReLoginError(err error) bool {
	switch err {
	case ErrNotLoggedIn, ErrSessionExpired, ErrLoginFailed:
		return true
	default:
		return false
	}
}

// ErrorCode 获取错误代码（用于AI判断错误类型）
func ErrorCode(err error) string {
	switch err {
	case ErrNotLoggedIn:
		return "NOT_LOGGED_IN"
	case ErrSessionExpired:
		return "SESSION_EXPIRED"
	case ErrLoginFailed:
		return "LOGIN_FAILED"
	case ErrNeedScanQRCode:
		return "NEED_SCAN_QRCODE"
	case ErrNeedPushConfirm:
		return "NEED_PUSH_CONFIRM"
	case ErrMessageNotFound:
		return "MESSAGE_NOT_FOUND"
	case ErrMessageTooLong:
		return "MESSAGE_TOO_LONG"
	case ErrMessageSendFailed:
		return "MESSAGE_SEND_FAILED"
	case ErrNoUnreadMessages:
		return "NO_UNREAD_MESSAGES"
	case ErrContactNotFound:
		return "CONTACT_NOT_FOUND"
	case ErrInvalidContactID:
		return "INVALID_CONTACT_ID"
	case ErrNetworkError:
		return "NETWORK_ERROR"
	case ErrWechatServerError:
		return "WECHAT_SERVER_ERROR"
	case ErrRateLimited:
		return "RATE_LIMITED"
	case ErrConfigNotFound:
		return "CONFIG_NOT_FOUND"
	case ErrInvalidConfig:
		return "INVALID_CONFIG"
	case ErrStorageError:
		return "STORAGE_ERROR"
	case ErrPermissionDenied:
		return "PERMISSION_DENIED"
	case ErrOperationNotAllowed:
		return "OPERATION_NOT_ALLOWED"
	default:
		return "UNKNOWN_ERROR"
	}
}
