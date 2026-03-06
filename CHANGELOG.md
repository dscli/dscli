# Changelog

所有 dscli 项目的显著变更都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
并且本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.5.3] - 2026-03-06

### 新增
- 添加段落系统详细设计文档（org mode格式）
- 添加 `segment` 命令系列，统一管理提示词段落
- 增加历史消息限制从8到32，提升对话连续性

### 变更
- 重构段落系统，统一使用"段落"概念，避免template和segment混淆
- 简化默认段落初始化逻辑
- 修复领域ID和段落查询逻辑
- 使用统一的Print函数，提升代码一致性

### 设计文档
- 创建 `docs/segment_design.org` 详细说明段落系统设计
- 明确段落、领域、模型的核心概念
- 详细说明数据库结构、模板变量、渲染流程
- 提供设计原则、最佳实践和故障排除指南

## [0.5.2] - 2026-03-05

### 新增
- 添加模板系统，支持动态模板变量替换
- 添加 `template` 命令系列，支持模板管理
- 支持数据库模板存储，可自定义系统提示词

### 变更
- 重构系统提示词生成逻辑，支持模板渲染
- 改进日期处理，所有日期相关功能使用动态日期
- 优化数据库连接管理，用后即关

### 修复
- 修复模板重复插入问题
- 修复模型ID常量定义
- 改进错误处理和回退机制

## [0.5.1] - 2026-02-28

### 变更
- 版本更新发布：从 v0.5.0 升级到 v0.5.1
- 更新 README.md 中的版本信息和安装说明

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

所有 dscli 项目的显著变更都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
并且本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.5.1] - 2026-02-28

### 变更
- 版本更新发布：从 v0.5.0 升级到 v0.5.1
- 更新 README.md 中的版本信息和安装说明

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

[0.5.1]: https://gitcode.com/dscli/dscli/compare/v0.5.0...v0.5.1
[0.5.0]: https://gitcode.com/dscli/dscli/compare/v0.4...v0.5.0
[0.4.0]: https://gitcode.com/dscli/dscli/compare/v0.3...v0.4
[0.3.0]: https://gitcode.com/dscli/dscli/compare/v0.2...v0.3
[0.2.0]: https://gitcode.com/dscli/dscli/compare/v0.1...v0.2
[0.1.0]: https://gitcode.com/dscli/dscli/releases/tag/0.1

文件信息:
- 路径: /home/nanjj/src/gitcode.com/dscli/dscli/CHANGELOG.md
- 大小: 2114 字节
权限: -rw-r--r--
修改时间: 2026-02-28 15:54:48

=== 执行统计 ===
执行时间: 94.807µs
状态: 成功