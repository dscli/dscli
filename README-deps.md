# dscli Python Parser 依赖管理

## 概述

dscli 使用嵌入式 Python 脚本来解析非 Go 语言的文件结构。为了确保解析器正常工作，需要检查和管理 Python 依赖。

## 依赖结构

### 必需依赖（内置）
- `json` - JSON 处理
- `re` - 正则表达式
- `ast` - Python AST 解析
- `typing` - 类型注解
- `traceback` - 错误追踪
- `importlib.util` - 模块导入工具

这些是 Python 标准库的一部分，通常不需要额外安装。

### 可选依赖（增强功能）
- `astroid>=3.0.0` - 增强的 Python AST 解析和类型推断
- `javalang>=0.13.0` - Java 语言解析
- `pycparser>=2.21` - C/C++ 解析

这些依赖提供更准确的解析功能，但不是必需的。

## 使用方法

### 1. 检查依赖状态

```bash
# 检查Python依赖
dscli deps

# 详细输出
dscli deps --verbose
```

### 2. 安装依赖

```bash
# 安装所有可选依赖
dscli deps --install

# 或使用setup模式（检查并安装）
dscli deps --setup
```

### 3. 解析文件时自动检查

当使用 `parse` 命令时，系统会自动检查依赖：

```bash
# 解析Python文件（自动检查依赖）
dscli parse example.py

# 详细输出，显示依赖信息
dscli parse example.py --verbose
```

### 4. 手动安装依赖

如果需要手动安装依赖：

```bash
# 使用pip安装
python3 -m pip install astroid javalang pycparser

# 或使用提供的setup.py
python3 setup.py install
```

## 依赖检查流程

1. **启动检查**：在调用 Python 解析器前，Go 代码会先执行依赖检查
2. **JSON 通信**：通过标准输入传递 JSON 数据给 Python 脚本
3. **依赖验证**：Python 脚本检查所有必需和可选依赖
4. **结果返回**：返回依赖状态和增强功能信息
5. **错误处理**：如果依赖不满足，提供清晰的错误信息和安装指导

## 文件结构

```
dscli/
├── parse.go              # Go解析器主文件（包含依赖检查）
├── parse.py              # Python解析器脚本（嵌入在Go二进制中）
├── deps_check.py         # Python依赖检查器
├── setup.py              # Python依赖安装脚本
├── requirements.txt      # 依赖列表
├── test_deps.py          # 依赖测试脚本
└── README-deps.md        # 本文档
```

## 测试依赖系统

```bash
# 运行依赖测试
python3 test_deps.py

# 测试setup脚本
python3 setup.py check
python3 setup.py test
```

## 故障排除

### 1. Python 未安装

```bash
# 检查Python版本
python3 --version

# 如果未安装，安装Python3
# Ubuntu/Debian
sudo apt-get install python3 python3-pip

# macOS
brew install python3
```

### 2. pip 未安装

```bash
# 检查pip
python3 -m pip --version

# 如果未安装，安装pip
# Ubuntu/Debian
sudo apt-get install python3-pip

# macOS (通常随Python一起安装)
```

### 3. 依赖安装失败

```bash
# 尝试升级pip
python3 -m pip install --upgrade pip

# 使用用户安装（避免权限问题）
python3 -m pip install --user astroid javalang pycparser
```

### 4. 特定依赖问题

```bash
# 检查特定依赖
python3 -c "import astroid; print(astroid.__version__)"

# 重新安装特定依赖
python3 -m pip install --force-reinstall astroid
```

## 开发说明

### 添加新依赖

1. 在 `deps_check.py` 的 `OPTIONAL_DEPS` 中添加新依赖
2. 更新 `requirements.txt`
3. 更新 `setup.py` 中的安装逻辑
4. 在 `parse.py` 中添加相应的增强功能检查

### 更新依赖版本

1. 修改 `requirements.txt` 中的版本号
2. 更新 `deps_check.py` 中的 `min_version`
3. 测试新版本兼容性

### 嵌入Python脚本

Python脚本通过 `go:embed` 嵌入到Go二进制中：

```go
//go:embed parse.py
var pythonScript string
```

这确保了Python解析器代码与Go二进制一起分发。

## 性能考虑

- 依赖检查只在第一次调用Python解析器时执行
- 检查结果会被缓存（在当前会话中）
- 依赖检查开销很小（<100ms）
- 可选依赖缺失不会阻止基本功能

## 安全考虑

- 依赖安装使用官方PyPI源
- 版本号固定以避免破坏性更新
- 用户可以选择不安装可选依赖
- 所有依赖都是开源且广泛使用的库