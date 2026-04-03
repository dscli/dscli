# dscli v0.7.3 发布说明

**发布日期**: 2026-04-04  
**Git提交**: b5a7469  
**上一个版本**: v0.7.2

## 🚀 主要变更

### 🔧 工具改进
1. **代码审查工具增强**
   - 修复Git状态检查逻辑：只检查已修改但未提交的变更，忽略未跟踪文件
   - 恢复详细的单元测试错误输出
   - 改进提交记录检查和错误处理
   - 优化常量使用方式

2. **Git工具优化**
   - 改进`-C`参数处理：从子命令改为独立参数
   - 根据专家建议优化Git工具实现

### 🛠️ 构建系统
1. **移除modernize工具**
   - 从Makefile的`gofmt`目标中移除`modernize`命令
   - 从`fmt-check`目标中移除`modernize`检查
   - 简化构建流程，减少外部依赖
   - `goimports`和`gofumpt`已提供足够的代码格式化功能

2. **修复版本号显示**
   - 修复Makefile中版本号显示重复"v"前缀的问题
   - 现在正确显示`v0.7.3`而不是`vv0.7.3`

### 🗑️ 清理工作
1. **移除sqlite工具**
   - 由于ShellExec重构后不再支持sqlite3，且使用频率低
   - 更新工具参考文档，标记sqlite工具已移除

### 🐛 Bug修复
1. **issue创建工具**
   - 修复API参数名和状态码检查问题

## 📦 安装方式

### 从源码安装
```bash
go install gitcode.com/dscli/dscli@v0.7.3
```

### 使用Makefile
```bash
make install
```

### 预编译二进制
支持以下平台：
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

## 🔄 从v0.7.2升级

这是一个向后兼容的版本，主要包含工具改进和清理工作。升级时无需特殊操作。

## 📋 已知问题

1. Git工具测试在某些环境下可能失败（与版本发布无关）
2. 代码审查工具的`user`参数问题需要进一步调查（不影响核心功能）

## 🙏 致谢

感谢所有贡献者和用户的支持！

---
**维护者**: Nan Jun Jie <nanjj@example.com>  
**项目地址**: gitcode.com/dscli/dscli  
**文档**: 项目根目录下的README.md和docs/目录