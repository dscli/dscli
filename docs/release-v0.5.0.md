# dscli v0.5.0 发布说明

## 版本信息
- **版本号**: v0.5.0
- **发布日期**: 2026年2月28日
- **Git 标签**: `v0.5.0`
- **上一个版本**: v0.4.0

## 版本亮点

🎉 **dscli v0.5.0 是一个功能完备的版本**，经过43个迭代的开发和优化，现在提供了完整的AI增强开发工具箱功能。

## 主要特性

### 1. 🆕 新增功能
- **版本管理**: 新增 `dscli version` 命令，显示版本信息和运行时环境
- **完整的版本历史**: 在README中记录所有版本变更
- **变更日志**: 创建CHANGELOG.md文件，遵循Keep a Changelog规范

### 2. 🤖 AI功能增强
- **智能对话**: 支持与DeepSeek AI进行自然语言对话
- **工具调用**: AI可以直接操作文件、执行Git命令、读写配置
- **代码补全**: FIM功能提供智能代码补全建议
- **模型管理**: 支持多种DeepSeek模型切换

### 3. 🔧 开发工具
- **Git Issue管理**: 完整的issue生命周期管理
  - `list` - 列出issue（支持状态筛选）
  - `show` - 查看issue详情
  - `create` - 创建新issue
  - `update` - 更新issue
- **技能管理**: 保存和复用常用prompt模板
- **对话历史**: 基于项目的上下文记忆

### 4. 🛠️ 实用工具
- **格式转换**: Markdown转Org模式格式
- **数据库支持**: SQLite存储所有配置和历史数据
- **Emacs集成**: 通过dscli.el提供Emacs原生体验
- **配置管理**: 统一的配置目录和环境管理

## 技术改进

### 架构优化
- **模块化设计**: 清晰的代码结构和职责分离
- **格式化系统**: 统一的消息格式化接口，支持多种输出模式
- **错误处理**: 增强的错误处理和日志记录机制

### 兼容性提升
- **Go 1.18+**: 使用`any`替代`interface{}`，提升现代Go兼容性
- **跨平台**: 支持Linux、macOS等主流操作系统
- **依赖管理**: 清晰的go.mod依赖声明

### 测试覆盖
- **单元测试**: 核心功能都有相应的测试用例
- **集成测试**: 确保各模块协同工作正常
- **代码质量**: 高测试覆盖率保证代码稳定性

## 安装方式

### 推荐安装
```bash
go install gitcode.com/dscli/dscli@v0.5.0
```

### 从源码构建
```bash
git clone https://gitcode.com/dscli/dscli.git
cd dscli
git checkout v0.5.0
make install
```

### 配置要求
1. DeepSeek API密钥（从[DeepSeek平台](https://platform.deepseek.com/)获取）
2. Go 1.18+ 环境

## 使用示例

### 查看版本信息
```bash
dscli version
```

### AI对话
```bash
echo "如何用Go实现HTTP服务器？" | dscli chat
```

### Git Issue管理
```bash
# 列出所有打开的issue
dscli issue list

# 创建新issue
dscli issue create
```

### 代码补全
```bash
echo "def fibonacci(n):" | dscli fim
```

## 向后兼容性

v0.5.0版本保持与之前版本的完全兼容性：
- 所有现有命令和选项保持不变
- 配置文件格式向后兼容
- API接口保持稳定

## 已知问题

暂无已知重大问题。如有发现，请在[Issues页面](https://gitcode.com/dscli/dscli/issues)报告。

## 贡献者

感谢所有为dscli项目做出贡献的开发者！

## 下一步计划

- 更多AI模型支持
- 插件系统开发
- 性能优化
- 文档完善

## 支持与反馈

- **项目地址**: https://gitcode.com/dscli/dscli
- **问题反馈**: https://gitcode.com/dscli/dscli/issues
- **文档**: README.md 和 CONTRIBUTE.md

---

**dscli v0.5.0** - 让命令行开发更智能、更高效！