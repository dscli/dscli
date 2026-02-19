package log

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// LogLevel 定义日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	currentLevel LogLevel = INFO
	logFile      *os.File
	logger       *log.Logger
)

func init() {
	// 确保日志目录存在
	logDir := filepath.Join(os.Getenv("HOME"), ".dscli")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建日志目录失败: %v\n", err)
		return
	}

	// 打开日志文件
	logPath := filepath.Join(logDir, "dscli.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开日志文件失败: %v\n", err)
		return
	}

	logFile = file
	
	// 创建logger，同时输出到文件和控制台（stderr）
	multiWriter := io.MultiWriter(os.Stderr, logFile)
	logger = log.New(multiWriter, "", 0) // 不使用前缀，我们自己控制格式
	
	// 记录初始化信息
	logger.Println(formatMessage("INFO", "日志系统初始化完成，日志文件: %s", logPath))
}

// Close 关闭日志文件
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

// SetLevel 设置日志级别
func SetLevel(level LogLevel) {
	currentLevel = level
}

// formatMessage 格式化日志消息
func formatMessage(level string, msg string, args ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	formattedMsg := fmt.Sprintf(msg, args...)
	return fmt.Sprintf("[%s] [%s] %s", timestamp, level, formattedMsg)
}

// logMessage 输出日志消息
func logMessage(level LogLevel, levelStr string, msg string, args ...interface{}) {
	if currentLevel <= level && logger != nil {
		logger.Println(formatMessage(levelStr, msg, args...))
	}
}

// Debug 输出调试日志
func Debug(msg string, args ...interface{}) {
	logMessage(DEBUG, "DEBUG", msg, args...)
}

// Info 输出信息日志
func Info(msg string, args ...interface{}) {
	logMessage(INFO, "INFO", msg, args...)
}

// Warn 输出警告日志
func Warn(msg string, args ...interface{}) {
	logMessage(WARN, "WARN", msg, args...)
}

// Error 输出错误日志
func Error(msg string, args ...interface{}) {
	logMessage(ERROR, "ERROR", msg, args...)
}

// APIRequest 记录API请求日志
func APIRequest(method, endpoint string, data interface{}) {
	if currentLevel <= DEBUG {
		Debug("API请求: %s %s", method, endpoint)
		if data != nil {
			jsonData, _ := json.MarshalIndent(data, "", "  ")
			Debug("请求数据: %s", string(jsonData))
		}
	}
}

// APIResponse 记录API响应日志
func APIResponse(statusCode int, data interface{}) {
	if currentLevel <= DEBUG {
		Debug("API响应: 状态码 %d", statusCode)
		if data != nil {
			jsonData, _ := json.MarshalIndent(data, "", "  ")
			Debug("响应数据: %s", string(jsonData))
		}
	}
}

// ToolCall 记录工具调用日志
func ToolCall(toolName string, args interface{}) {
	Info("执行工具: %s", toolName)
	if currentLevel <= DEBUG && args != nil {
		jsonArgs, _ := json.MarshalIndent(args, "", "  ")
		Debug("工具参数: %s", string(jsonArgs))
	} else if currentLevel <= INFO && args != nil {
		// 在INFO级别显示简化的参数信息
		if argsMap, ok := args.(map[string]interface{}); ok {
			var paramStr string
			for k, v := range argsMap {
				if strVal, ok := v.(string); ok && len(strVal) > 50 {
					paramStr += fmt.Sprintf("%s:%.50s... ", k, strVal)
				} else {
					paramStr += fmt.Sprintf("%s:%v ", k, v)
				}
			}
			if paramStr != "" {
				Info("工具参数: %s", paramStr)
			}
		}
	}
}

// ToolResult 记录工具执行结果日志
func ToolResult(toolName string, result string, err error) {
	if err != nil {
		Error("工具执行失败: %s - %v", toolName, err)
	} else {
		Info("工具执行完成: %s", toolName)
		if currentLevel <= DEBUG && result != "" {
			Debug("工具结果: %s", result)
		} else if currentLevel <= INFO && result != "" {
			// 在INFO级别显示简化的结果
			if len(result) > 100 {
				Info("结果: %.100s...", result)
			} else {
				Info("结果: %s", result)
			}
		}
	}
}

// ChatMessage 记录聊天消息日志
func ChatMessage(role, content string, toolCalls interface{}) {
	if role == "user" {
		Info("用户消息")
		if currentLevel <= INFO && content != "" {
			if len(content) > 100 {
				Info("内容: %.100s...", content)
			} else {
				Info("内容: %s", content)
			}
		}
	} else if role == "assistant" {
		if toolCalls != nil {
			Info("助手回复（包含工具调用）")
		} else {
			Info("助手回复")
			if currentLevel <= INFO && content != "" {
				if len(content) > 100 {
					Info("内容: %.100s...", content)
				} else {
					Info("内容: %s", content)
				}
			}
		}
	} else {
		Info("聊天消息: %s", role)
	}
	
	if currentLevel <= DEBUG {
		if content != "" {
			Debug("完整内容: %s", content)
		}
		if toolCalls != nil {
			jsonToolCalls, _ := json.MarshalIndent(toolCalls, "", "  ")
			Debug("工具调用详情: %s", string(jsonToolCalls))
		}
	}
}

// DatabaseOperation 记录数据库操作日志
func DatabaseOperation(operation string, args ...interface{}) {
	if currentLevel <= DEBUG {
		Debug("数据库操作: %s", operation)
		if len(args) > 0 {
			Debug("操作参数: %v", args)
		}
	}
}

// FileOperation 记录文件操作日志
func FileOperation(operation, path string, args ...interface{}) {
	Info("文件操作: %s - %s", operation, path)
	if currentLevel <= DEBUG && len(args) > 0 {
		Debug("操作参数: %v", args)
	}
}

// GitOperation 记录Git操作日志
func GitOperation(operation string, args ...interface{}) {
	Info("Git操作: %s", operation)
	if currentLevel <= DEBUG && len(args) > 0 {
		Debug("操作参数: %v", args)
	}
}
