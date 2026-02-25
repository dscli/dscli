# dscli - DeepSeek 编程助手命令行工具
2025-03-15

## 简介
dscli 是一个命令行编程助手，后接DeepSeek API，帮助您开发代码，也有一定的设计能力。你可以直接在命令行终端使用，也可以集成到Emacs使用（通过dscli.el，或自己集成）。

项目地址：[gitcode.com/dscli/dscli](https://gitcode.com/dscli/dscli)

## 快速开始
### 安装
```bash
go install gitcode.com/dscli/dscli@latest
   ```
或使用 Makefile：
```bash
git clone https://gitcode.com/dscli/dscli.git
cd dscli
make install    # 安装到 $GOPATH/bin
``` 

## 配置
设置 DeepSeek API 密钥：
```bash
export DEEPSEEK_API_KEY="your-api-key-here"
```

## 核心功能：```dscli chat```
dscli chat 是主要功能，用于与 DeepSeek 编程助手对话。

### 基本使用
切到项目目录（比如dscli），通过标准输入发送问题：
```bash
cd dscli
dscli chat <<EOF
如何用Go实现HTTP服务器？
EOF
```

## 常用场景示例
### 代码编写
```bash
dscli chat <<EOF
写一个Python函数计算斐波那契数列
EOF
```

### 代码解释
```bash
dscli chat <<EOF
解释这段代码的作用：$(cat complex_code.go)
EOF
```

### 技术问题
```bash
dscli chat <<EOF
Docker和Kubernetes有什么区别？
EOF
```

## 选择模型
使用不同的 DeepSeek 模型：
```bash
echo "问题内容" | dscli chat --model deepseek-reasoner
```

可用模型：
```bash
dscli models
```

示例输出：
```bash
deepseek-chat        # 通用聊天
deepseek-reasoner    # 复杂推理
```

## Emacs 集成 (dscli.el)
在 Emacs 中使用 dscli 更加方便：

1. 安装 dscli.el：
   ```emacs-lisp
   (add-to-list 'load-path "/path/to/dscli.el")
   (require 'dscli)
   ```

2. 基本使用：
   - ```M-x dscli-chat``` 启动聊天
   - 在临时缓冲区输入问题
   - 按 ```C-c C-c``` 发送
   - 查看 org mode 格式的回答

## 其他功能
### 查询余额
查看 API 使用情况：
```bash
dscli balance
```

### 代码补全 (FIM)
使用代码补全功能：
```bash
echo "def fibonacci(n):" | dscli fim
```

## 高级配置
### 环境变量
- ```DEEPSEEK_API_KEY```：API 密钥（必需）
- ```DEEPSEEK_BASE_URL```：API 地址（可选，默认 https://api.deepseek.com）

## 命令行参数
所有命令支持：
- ```--api-ke```：指定 API 密钥
- ```--base-ur```：指定 API 地址
- ```--debu```：调试模式

## 常见问题
### 如何获得 DeepSeek API 密钥？
访问 [DeepSeek 平台](https://platform.deepseek.com/) 注册并获取 API 密钥。

### 支持哪些编程语言？
DeepSeek 支持所有主流编程语言，包括 Go、Python、JavaScript、Java、C++ 等。

## 许可证
Apache License 2.0
