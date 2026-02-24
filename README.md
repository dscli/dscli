# dscli - DeepSeek 命令行工具
2025-03-15

## 简介
```dscli``` 是一个用 Go 语言编写的命令行工具，用于与 DeepSeek API 进行交互。它支持以下四个核心功能：
- 列出模型 (```models```)
- 查询余额 (```balance```)
- 对话聊天 (```chat```)
- FIM 代码补全 (```fim```)

项目地址：[gitcode.com/nanjunjie/dscli](https://gitcode.com/nanjunjie/dscli)

## 环境变量
在使用```dscli```之前，需要设置以下环境变量：
- ```DEEPSEEK_API_KEY``` ：你的 DeepSeek API 密钥（必填）
- ```DEEPSEEK_BASE_URL``` ：API 基础地址（可选，默认为 **https://api.deepseek.com**）

你也可以通过命令行标志 ```--api-key``` 和 ```--base-url``` 临时指定，优先级高于环境变量。

## 编译
### 使用 Go 直接编译
确保已安装 Go 1.21 或更高版本，然后执行：
```bash
go mod tidy
go build -o dscli
```

### 使用 Makefile（推荐）
项目提供了 Makefile，支持常见操作：
```bash
make build      # 编译到 build/dscli
make install    # 安装到 $GOPATH/bin
make clean      # 清理构建产物
make test       # 运行测试（如果有）
```

还支持交叉编译示例：
```bash
make build-linux   # Linux amd64
make build-windows # Windows amd64
make build-macos   # macOS (amd64 + arm64)
```

编译后的二进制文件会存放在 ```build/``` 目录下。

## 使用说明
### 全局标志
所有子命令都支持以下全局标志：
- ```--api-key```：指定 API 密钥
- ```--base-url```：指定 API 基础 URL
- ```--debug```：启用调试模式，打印请求和响应详情

### 子命令
#### ```models```

列出 DeepSeek 支持的所有模型。
```bash
dscli models
```

示例输出：
```bash
ID                  对象      拥有者
deepseek-chat       model    deepseek
deepseek-reasoner   model    deepseek
```

#### ```balance```
查询账户余额信息。
```bash
dscli balance
```

示例输出：
```bash
货币   总余额   赠送余额   充值余额
CNY    100.00  50.00      50.00
```

#### ```chat```
与 DeepSeek 聊天模型进行多轮对话，并支持工具调用（文件操作、Git）。

**消息输入**：通过标准输入提供消息内容。

**会话隔离**：每个项目目录（Git 仓库根或当前目录）拥有独立的对话历史。

**工具列表**：
- ```read_file```：读取文件内容
- ```write_fil```：写入文件（自动创建目录）
- ```search_file```：按文件名模式或内容搜索文件
- ```git_ad```：将文件添加到 Git 暂存区
- ```git_commi```：提交暂存区更改
- ```git_lo```：查看提交历史
- ```git_dif```：查看差异
- ```git_statu```：查看仓库状态

**示例**：
```bash
# 创建文件
echo "创建一个 main.go 文件，内容为 package main\nfunc main() { println(\"hello\") }" | dscli chat

# 搜索包含 "TODO" 的文件
echo "在项目中搜索包含 'TODO' 的文件" | dscli chat

# Git 操作
echo "把当前所有修改添加到 Git 并提交，信息为 'update'" | dscli chat
```

#### ```fim```
FIM 代码补全，适用于代码生成场景。
```bash
dscli fim "def hello():" --suffix "    print('world')"
echo "function add(a, b) {" | dscli fim
```

可用标志：
- ```--model```：模型名称（默认 ```deepseek-chat```）
- ```--suffi```：补全后缀（可选）
- ```--max-token```：最大生成 token 数（默认 1024）
- ```--temperatur```：采样温度（默认 0.7）

## 示例汇总
```bash
# 设置环境变量（建议写入 ~/.bashrc）
export DEEPSEEK_API_KEY=your_key_here
export DEEPSEEK_BASE_URL=https://api.deepseek.com

# 查看模型列表
dscli models

# 查询余额
dscli balance

# 聊天
dscli chat "用 Python 写一个快速排序"

# 代码补全
dscli fim "import numpy as np" --suffix "print(np.array([1,2,3]))"

# 调试模式查看请求
dscli --debug models
```

## 贡献
欢迎提交 Issue 和 PR。请确保代码符合 Go 标准格式并包含必要的测试。

## 许可证
[MIT](LICENSE)
