# Changelog

所有 dscli 项目的显著变更都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
并且本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.5.0] - 2026-02-28

### 新增
- 添加 `version` 命令，显示版本信息和运行时信息
- 在 main.go 中添加版本常量定义
- 更新 README.md 包含版本历史和安装说明

### 变更
- 重构格式化系统，支持多种输出模式
- 改进工具调用接口，增强稳定性
- 优化错误处理和日志记录

### 修复
- 修复 Printf 函数相关问题
- 移除不再使用的 --mode 特性
- 修复代码中的兼容性问题

### 技术改进
- 将 interface{} 替换为 any，提升 Go 1.18+ 兼容性
- 增强单元测试覆盖率
- 改进代码结构和模块化设计

## [0.4.0] - 2026-02-27

### 新增
- 格式化系统重构，支持 markdown 和 org 模式输出
- 添加统一的格式化接口
- 增强输出模式支持

### 变更
- 重构格式化系统架构
- 改进输出处理流程

## [0.3.0] - 2026-02-26

### 新增
- Git issue 管理功能
  - `issue list` - 列出 issue
  - `issue show` - 查看 issue 详情
  - `issue create` - 创建新 issue
  - `issue update` - 更新 issue
- 支持 open/closed/all 状态筛选

### 变更
- 增强数据库支持
- 改进项目结构

## [0.2.0] - 2026-02-25

### 新增
- 增强 AI 工具调用能力
- 添加技能管理系统
- 支持对话历史存储

### 变更
- 改进命令行接口
- 优化配置管理

## [0.1.0] - 2026-02-24

### 新增
- 初始版本发布
- 基础 AI 对话功能
- 支持 models、balance、chat、fim 命令
- 基本配置和日志系统

[0.5.0]: https://gitcode.com/dscli/dscli/compare/v0.4...v0.5.0
[0.4.0]: https://gitcode.com/dscli/dscli/compare/v0.3...v0.4
[0.3.0]: https://gitcode.com/dscli/dscli/compare/v0.2...v0.3
[0.2.0]: https://gitcode.com/dscli/dscli/compare/v0.1...v0.2
[0.1.0]: https://gitcode.com/dscli/dscli/releases/tag/0.1