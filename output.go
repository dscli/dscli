package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// LogLevel 定义日志级别（保留类型但不再使用多级过滤）
type LogLevel int

const (
	// LogLevelDebug 调试级别
	LogLevelDebug LogLevel = iota
	// LogLevelInfo 信息级别
	LogLevelInfo
	// LogLevelWarn 警告级别
	LogLevelWarn
	// LogLevelError 错误级别
	LogLevelError
	// LogLevelFatal 致命级别
	LogLevelFatal
)

func (logLevel LogLevel) String() string {
	switch logLevel {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	case LogLevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// 输出系统变量
var (
	// 是否启用颜色输出
	outputColorEnabled bool = true

	// 是否显示时间戳
	outputShowTimestamp bool = true

	// 是否显示详细输出
	outputVerbose bool = false

	// 输出写入器
	outputWriter io.Writer = os.Stdout

	// 错误输出写入器
	outputErrorWriter io.Writer = os.Stderr

	// 输出格式
	outputMode string = "markdown"

	// Markdown转换器
	markdown = NewMarkdownToOrgConverter()
)

// Color 定义颜色常量
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorGray   = "\033[90m"

	// 粗体颜色
	ColorBoldRed    = "\033[1;31m"
	ColorBoldGreen  = "\033[1;32m"
	ColorBoldYellow = "\033[1;33m"
	ColorBoldBlue   = "\033[1;34m"
	ColorBoldPurple = "\033[1;35m"
	ColorBoldCyan   = "\033[1;36m"
	ColorBoldWhite  = "\033[1;37m"
)

// Println 输出一行文本（保持向后兼容）
// Println 输出一行文本（保持向后兼容）
func Println(a ...any) (n int, err error) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	if outputMode == "org" {
		input := fmt.Sprintln(a...)
		err = markdown.ConvertLines(input, outputWriter)
		return
	}
	return fmt.Fprintln(outputWriter, a...)
}

// Printf 输出格式化文本（保持向后兼容）
// Printf 输出格式化文本（保持向后兼容）
func Printf(format string, a ...any) (n int, err error) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	if outputMode == "org" {
		input := fmt.Sprintf(format, a...)
		err = markdown.ConvertLines(input, outputWriter)
		return
	}
	return fmt.Fprintf(outputWriter, format, a...)
}

// SetOutputWriter 设置输出写入器
func SetOutputWriter(w io.Writer) {
	outputWriter = w
}

// SetOutputMode 设置输出模式
func SetOutputMode(mode string) {
	outputMode = mode
}

// SetColorEnabled 设置是否启用颜色输出
func SetColorEnabled(enabled bool) {
	outputColorEnabled = enabled
}

// SetShowTimestamp 设置是否显示时间戳
func SetShowTimestamp(show bool) {
	outputShowTimestamp = show
}

// SetVerbose 设置是否显示详细输出
func SetVerbose(verbose bool) {
	outputVerbose = verbose
}

// SetErrorWriter 设置错误输出写入器
func SetErrorWriter(w io.Writer) {
	outputErrorWriter = w
}

// colorize 根据是否启用颜色返回带颜色的字符串
func colorize(color, text string) string {
	if outputColorEnabled {
		return color + text + ColorReset
	}
	return text
}

// getTimestamp 获取时间戳字符串
func getTimestamp() string {
	if outputShowTimestamp {
		return time.Now().Format("2006-01-02 15:04:05")
	}
	return ""
}

// formatMessage 格式化消息
func formatMessage(level, color, message string) string {
	timestamp := getTimestamp()
	if timestamp != "" {
		return fmt.Sprintf("%s [%s] %s", timestamp, colorize(color, level), message)
	}
	return fmt.Sprintf("[%s] %s", colorize(color, level), message)
}

func JSONMarshal(v any) ([]byte, error) {
	if outputVerbose {
		return json.MarshalIndent(v, "", "  ")
	}
	return json.Marshal(v)
}

// DebugBytes output bytes if debug
// DebugBytes output bytes if debug
// DebugBytes output bytes if debug
func DebugBytes(lang string, b []byte) {
	if outputVerbose {
		Printf("```%s\n", lang)
		Println(string(b))
		Println("```")
	}
}

// Debug 输出调试信息（仅在verbose模式下显示）
// Debug 输出调试信息（仅在verbose模式下显示）
func Debug(format string, a ...any) {
	if outputVerbose {
		message := fmt.Sprintf(format, a...)
		formatted := formatMessage("DEBUG", ColorGray, message)
		Println(formatted)
	}
}

// Info 输出普通信息（始终显示）
// Info 输出普通信息（始终显示）
// Info 输出普通信息（始终显示）
func Info(format string, a ...any) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	message := fmt.Sprintf(format, a...)
	formatted := formatMessage("INFO", ColorGreen, message)
	Println(formatted)
}

// Warn 输出警告信息（始终显示）
func Warn(format string, a ...any) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	message := fmt.Sprintf(format, a...)
	formatted := formatMessage("WARN", ColorYellow, message)
	fmt.Fprintln(outputErrorWriter, formatted)
}

// Error 输出错误信息（始终显示）
func Error(format string, a ...any) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	message := fmt.Sprintf(format, a...)
	formatted := formatMessage("ERROR", ColorRed, message)
	fmt.Fprintln(outputErrorWriter, formatted)
}

// Fatal 输出致命错误信息并退出（始终显示）
func Fatal(format string, a ...any) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	message := fmt.Sprintf(format, a...)
	formatted := formatMessage("FATAL", ColorBoldRed, message)
	fmt.Fprintln(outputErrorWriter, formatted)
	os.Exit(1)
}

// Success 输出成功信息
func Success(format string, a ...any) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	message := fmt.Sprintf(format, a...)
	formatted := colorize(ColorBoldGreen, "✓ "+message)
	Println(formatted)
}

// Notice 输出注意信息
func Notice(format string, a ...any) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	message := fmt.Sprintf(format, a...)
	formatted := colorize(ColorBoldCyan, "→ "+message)
	Println(formatted)
}

// PrintHeader 输出标题
func PrintHeader(title string) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	line := strings.Repeat("=", len(title)+4)
	Println(colorize(ColorBoldCyan, line))
	Println(colorize(ColorBoldCyan, "  "+title+"  "))
	Println(colorize(ColorBoldCyan, line))
}

// PrintSection 输出章节标题
func PrintSection(title string) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	Println()
	Println(colorize(ColorBoldBlue, "▶ "+title))
	Println(colorize(ColorGray, strings.Repeat("─", len(title)+2)))
}

// PrintSubSection 输出子章节标题
func PrintSubSection(title string) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	Println()
	Println(colorize(ColorBoldPurple, "  • "+title))
}

// PrintBullet 输出项目符号
func PrintBullet(text string) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	Println(colorize(ColorWhite, "  ◦ "+text))
}

// PrintKeyValue 输出键值对
func PrintKeyValue(key, value string) {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	Printf("%s: %s\n",
		colorize(ColorBoldWhite, key),
		colorize(ColorCyan, value))
}

// PrintJSON 输出JSON格式数据
func PrintJSON(data any) error {
	// 记录输出事件，重置等待计时器
	manager := GetWaitingManager()
	if manager != nil && manager.IsActive() {
		manager.RecordOutput()
	}

	jsonStr, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	Println(colorize(ColorGray, string(jsonStr)))
	return nil
}

// ProgressBar 进度条结构
type ProgressBar struct {
	total     int64
	current   int64
	width     int
	showValue bool
	startTime time.Time
}

// NewProgressBar 创建新的进度条
func NewProgressBar(total int64) *ProgressBar {
	return &ProgressBar{
		total:     total,
		current:   0,
		width:     50,
		showValue: true,
		startTime: time.Now(),
	}
}

// SetWidth 设置进度条宽度
func (pb *ProgressBar) SetWidth(width int) {
	pb.width = width
}

// SetShowValue 设置是否显示数值
func (pb *ProgressBar) SetShowValue(show bool) {
	pb.showValue = show
}

// Update 更新进度
func (pb *ProgressBar) Update(current int64) {
	pb.current = current
	pb.render()
}

// Increment 增加进度
func (pb *ProgressBar) Increment(delta int64) {
	pb.current += delta
	if pb.current > pb.total {
		pb.current = pb.total
	}
	pb.render()
}

// render 渲染进度条
func (pb *ProgressBar) render() {
	if pb.total <= 0 {
		return
	}

	percent := float64(pb.current) / float64(pb.total) * 100
	filled := int(float64(pb.width) * float64(pb.current) / float64(pb.total))
	empty := pb.width - filled

	// 构建进度条字符串
	bar := colorize(ColorGreen, strings.Repeat("█", filled)) +
		colorize(ColorGray, strings.Repeat("░", empty))

	// 构建信息字符串
	info := fmt.Sprintf(" %.1f%%", percent)

	if pb.showValue {
		info = fmt.Sprintf(" %d/%d (%.1f%%)", pb.current, pb.total, percent)
	}

	// 计算耗时
	elapsed := time.Since(pb.startTime)
	info += fmt.Sprintf(" [%v]", elapsed.Round(time.Second))

	// 输出进度条（使用回车符覆盖上一行）
	Printf("\r%s %s", bar, info)
	// 如果完成，输出换行
	if pb.current >= pb.total {
		Println()
	}
}

// Finish 完成进度条
func (pb *ProgressBar) Finish() {
	if pb.current < pb.total {
		pb.current = pb.total
	}
	pb.render()
}

// Spinner 加载动画结构
type Spinner struct {
	frames    []string
	interval  time.Duration
	message   string
	stopChan  chan bool
	stopped   bool
	startTime time.Time
}

// NewSpinner 创建新的加载动画
func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		interval: 100 * time.Millisecond,
		message:  message,
		stopChan: make(chan bool),
		stopped:  false,
	}
}

// Start 开始加载动画
func (s *Spinner) Start() {
	s.startTime = time.Now()
	go s.run()
}

// run 运行动画
func (s *Spinner) run() {
	i := 0
	for {
		select {
		case <-s.stopChan:
			return
		default:
			frame := s.frames[i%len(s.frames)]
			elapsed := time.Since(s.startTime).Round(time.Second)
			Printf("\r%s %s [%v]",
				colorize(ColorYellow, frame),
				s.message,
				elapsed)
			time.Sleep(s.interval)
			i++
		}
	}
}

// Stop 停止加载动画
func (s *Spinner) Stop() {
	if s.stopped {
		return
	}
	s.stopped = true
	s.stopChan <- true
	// 清除动画行
	Printf("\r%s\r", strings.Repeat(" ", 80))
}

// StopWithMessage 停止加载动画并显示消息
func (s *Spinner) StopWithMessage(message string, success bool) {
	s.Stop()
	if success {
		Success("%s", message)
	} else {
		Error("%s", message)
	}
}

// IsVerbose 检查是否启用详细输出
func IsVerbose() bool {
	return outputVerbose
}
