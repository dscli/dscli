# dscli 配置说明

## 配置文件位置

dscli 支持以下位置的配置文件（按优先级排序）：

1. `~/.dscli/.env` （推荐，标准格式）
2. `~/.dscli/dscli.env` （兼容旧格式）
3. 当前目录的 `.env` 文件

## 配置文件格式

### 标准格式（推荐）
在 `~/.dscli/.env` 文件中：

```env
# DeepSeek API 配置
DEEPSEEK_API_KEY=your_api_key_here
DEEPSEEK_BASE_URL=https://api.deepseek.com/beta

# 可选配置
# LOG_LEVEL=info
# LOG_FORMAT=text
```

### 旧格式（兼容）
在 `~/.dscli/dscli.env` 文件中：

```bash
export DEEPSEEK_API_KEY="your_api_key_here"
export DEEPSEEK_BASE_URL="https://api.deepseek.com/beta"
```

## 环境变量

### 必需的环境变量
- `DEEPSEEK_API_KEY`: DeepSeek API 密钥

### 可选的环境变量
- `DEEPSEEK_BASE_URL`: API 基础 URL，默认为 `https://api.deepseek.com/beta`
- `LOG_LEVEL`: 日志级别，如 `debug`、`info`、`warn`、`error`
- `LOG_FORMAT`: 日志格式，如 `text`、`json`

## 配置加载顺序

1. 程序自动加载配置文件（无需在 `.bashrc` 中设置）
2. 已存在的环境变量优先级更高（不会被配置文件覆盖）
3. 如果没有找到配置文件，会显示提示信息

## 迁移说明

如果你之前已经在 `.bashrc` 中加载了配置：

```bash
# 旧的配置方式（可以删除）
source ~/.dscli/dscli.env
```

现在可以删除这行，dscli 会自动加载配置。

## 创建配置文件

首次使用时，可以运行以下命令创建配置文件：

```bash
mkdir -p ~/.dscli
cat > ~/.dscli/.env << 'EOF'
DEEPSEEK_API_KEY=your_api_key_here
DEEPSEEK_BASE_URL=https://api.deepseek.com/beta
EOF
```

## 验证配置

运行以下命令验证配置是否正确加载：

```bash
dscli --help
```

如果看到 "✅ 已加载配置文件: ..." 的提示，说明配置加载成功。