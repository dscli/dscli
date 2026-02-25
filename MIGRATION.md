# 项目迁移说明

## 迁移信息
- **原仓库**: gitcode.com/nanjunjie/dscli
- **新仓库**: gitcode.com/dscli/dscli
- **迁移时间**: $(date)
- **迁移原因**: 从个人仓库迁移到组织仓库，便于项目持续发展和团队协作

## 更新内容

### 1. 代码文件更新
- `go.mod`: 模块路径更新
  ```diff
  - module gitcode.com/nanjunjie/dscli
  + module gitcode.com/dscli/dscli
  ```

### 2. 文档更新
- `README.md`: 项目地址、安装命令、克隆命令
- `README.org`: Org-mode格式的文档
- `CONTRIBUTE.org`: 贡献指南中的仓库链接

### 3. Git配置更新
- 远程仓库地址更新为新的组织仓库

## 影响范围

### 开发者
1. **本地开发环境**:
   ```bash
   # 更新远程仓库
   git remote set-url origin https://gitcode.com/dscli/dscli.git
   
   # 或者重新克隆
   git clone https://gitcode.com/dscli/dscli.git
   ```

2. **Go模块依赖**:
   ```bash
   # 更新go.mod中的模块路径
   go mod edit -module gitcode.com/dscli/dscli
   ```

### 用户
1. **安装命令**:
   ```bash
   # 旧命令
   go install gitcode.com/nanjunjie/dscli@latest
   
   # 新命令
   go install gitcode.com/dscli/dscli@latest
   ```

2. **文档链接**: 所有文档中的链接已更新

## 验证检查

### 编译验证
```bash
go build ./...  # 应该成功编译
```

### 测试验证
```bash
go test ./...   # 所有测试应该通过
```

### 功能验证
- [x] 代码格式化功能正常
- [x] Git Hook正常工作
- [x] 数据库功能正常
- [x] API调用正常

## 回滚方案

如果需要回滚到原仓库：

1. **恢复远程仓库**:
   ```bash
   git remote set-url origin https://gitcode.com/nanjunjie/dscli.git
   ```

2. **恢复go.mod**:
   ```bash
   git checkout go.mod
   ```

3. **恢复文档**:
   ```bash
   git checkout README.md README.org CONTRIBUTE.org
   ```

## 后续步骤

1. **通知贡献者**: 更新仓库地址
2. **更新CI/CD**: 如果有CI/CD流水线，更新仓库配置
3. **更新包管理**: 如果有发布到包管理器，更新发布配置
4. **监控问题**: 关注是否有因迁移导致的问题

## 联系方式

如有问题，请联系项目维护团队。

---
*本文档由迁移脚本自动生成*
