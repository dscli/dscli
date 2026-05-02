#!/usr/bin/env python
"""
dscli-get-file-structure - Python implementation for file structure parsing

This script is embedded in the Go binary and executed via stdin.
It provides advanced file structure parsing capabilities for various languages.
"""

import sys
import json
import ast
import re
from typing import Dict, List, Any
import traceback
import importlib.util

class FileStructureParser:
    """Main parser class for analyzing file structure"""

    def __init__(self):
        self.language_parsers = {
            'go': self.parse_go,
            'python': self.parse_python,
            'javascript': self.parse_javascript,
            'typescript': self.parse_typescript,
            'java': self.parse_java,
            'c': self.parse_c,
            'cpp': self.parse_cpp,
            'markdown': self.parse_markdown,
            'org': self.parse_org,
            'elisp': self.parse_elisp,
            'makefile': self.parse_makefile,
            'cmake': self.parse_cmake,
            'vimscript': self.parse_vimscript,
            'vim': self.parse_vimscript,  # alias for .vim files
        }
        # Check dependencies
        self.deps_ok = self._check_dependencies()
        self.enhanced_capabilities = self._get_enhanced_capabilities()

    def parse(self, content: str, language: str) -> Dict[str, Any]:
        """Parse content with specified language"""
        if language not in self.language_parsers:
            return {
                'error': f"Unsupported language: {language}",
                'supported_languages': list(self.language_parsers.keys()),
                'errors': []
            }

        try:
            return self.language_parsers[language](content)
        except Exception as e:
            return {
                'error': f"Parsing error: {str(e)}",
                'errors': [f"Parsing error: {str(e)}"]
            }
    def parse_go(self, content: str) -> Dict[str, Any]:
        """Parse Go file structure using regex (Go AST parsing is done in Go)"""
        result = {
            'functions': [],
            'imports': [],
            'structs': [],
            'interfaces': [],
            'errors': []
        }

        try:
            lines = content.split('\n')

            # Parse imports
            for i, line in enumerate(lines):
                # 检查单行导入
                single_import_match = re.match(r'^\s*import\s+"([^"]+)"', line)
                if single_import_match:
                    result['imports'].append({
                        'name': single_import_match.group(1),
                        'type': 'import',
                        'lineno': i + 1
                    })

                # 检查多行导入开始
                multi_import_match = re.match(r'^\s*import\s*\(', line)
                if multi_import_match:
                    # 查找多行导入的结束
                    for j in range(i + 1, len(lines)):
                        if lines[j].strip() == ')':
                            # 处理多行导入中的每一行
                            for k in range(i + 1, j):
                                import_line = lines[k].strip()
                                if import_line and not import_line.startswith('//'):
                                    if import_line.startswith('"'):
                                        result['imports'].append({
                                            'name': import_line.strip('"'),
                                            'type': 'import',
                                            'lineno': k + 1
                                        })
                            break

            # Parse functions
            func_pattern = r'func\s+(?:\([^)]+\)\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)\s*(?:\([^)]*\))?\s*(?:\{[^}]*\})?'
            for i, line in enumerate(lines):
                # 查找函数定义
                func_match = re.search(func_pattern, line)
                if func_match:
                    # 查找函数体的结束
                    brace_count = line.count('{') - line.count('}')
                    if brace_count > 0:
                        # 函数体在同一行开始
                        for j in range(i + 1, len(lines)):
                            brace_count += lines[j].count('{') - lines[j].count('}')
                            if brace_count <= 0:
                                result['functions'].append({
                                    'name': func_match.group(1),
                                    'type': 'function',
                                    'lineno': i + 1,
                                    'end_lineno': j + 1
                                })
                                break
                    else:
                        # 函数体可能在后几行开始
                        for j in range(i + 1, min(i + 10, len(lines))):
                            if '{' in lines[j]:
                                # 找到函数体开始
                                brace_count = 1
                                for k in range(j + 1, len(lines)):
                                    brace_count += lines[k].count('{') - lines[k].count('}')
                                    if brace_count <= 0:
                                        result['functions'].append({
                                            'name': func_match.group(1),
                                            'type': 'function',
                                            'lineno': i + 1,
                                            'end_lineno': k + 1
                                        })
                                        break
                                break
                        else:
                            # 没有找到函数体，可能只有声明
                            result['functions'].append({
                                'name': func_match.group(1),
                                'type': 'function',
                                'lineno': i + 1
                            })

            # Parse structs
            struct_pattern = r'type\s+([A-Za-z_][A-Za-z0-9_]*)\s+struct\s*\{'
            for i, line in enumerate(lines):
                struct_match = re.search(struct_pattern, line)
                if struct_match:
                    # 查找结构体的结束
                    brace_count = line.count('{') - line.count('}')
                    if brace_count > 0:
                        # 结构体在同一行开始
                        for j in range(i + 1, len(lines)):
                            brace_count += lines[j].count('{') - lines[j].count('}')
                            if brace_count <= 0:
                                result['structs'].append({
                                    'name': struct_match.group(1),
                                    'type': 'struct',
                                    'lineno': i + 1,
                                    'end_lineno': j + 1
                                })
                                break
                    else:
                        # 结构体可能在后几行开始
                        for j in range(i + 1, min(i + 10, len(lines))):
                            if '{' in lines[j]:
                                # 找到结构体开始
                                brace_count = 1
                                for k in range(j + 1, len(lines)):
                                    brace_count += lines[k].count('{') - lines[k].count('}')
                                    if brace_count <= 0:
                                        result['structs'].append({
                                            'name': struct_match.group(1),
                                            'type': 'struct',
                                            'lineno': i + 1,
                                            'end_lineno': k + 1
                                        })
                                        break
                                break
                        else:
                            # 没有找到结构体定义
                            result['structs'].append({
                                'name': struct_match.group(1),
                                'type': 'struct',
                                'lineno': i + 1
                            })

            # Parse interfaces
            interface_pattern = r'type\s+([A-Za-z_][A-Za-z0-9_]*)\s+interface\s*\{'
            for i, line in enumerate(lines):
                interface_match = re.search(interface_pattern, line)
                if interface_match:
                    # 查找接口的结束
                    brace_count = line.count('{') - line.count('}')
                    if brace_count > 0:
                        # 接口在同一行开始
                        for j in range(i + 1, len(lines)):
                            brace_count += lines[j].count('{') - lines[j].count('}')
                            if brace_count <= 0:
                                result['interfaces'].append({
                                    'name': interface_match.group(1),
                                    'type': 'interface',
                                    'lineno': i + 1,
                                    'end_lineno': j + 1
                                })
                                break
                    else:
                        # 接口可能在后几行开始
                        for j in range(i + 1, min(i + 10, len(lines))):
                            if '{' in lines[j]:
                                # 找到接口开始
                                brace_count = 1
                                for k in range(j + 1, len(lines)):
                                    brace_count += lines[k].count('{') - lines[k].count('}')
                                    if brace_count <= 0:
                                        result['interfaces'].append({
                                            'name': interface_match.group(1),
                                            'type': 'interface',
                                            'lineno': i + 1,
                                            'end_lineno': k + 1
                                        })
                                        break
                                break
                        else:
                            # 没有找到接口定义
                            result['interfaces'].append({
                                'name': interface_match.group(1),
                                'type': 'interface',
                                'lineno': i + 1
                            })

        except Exception as e:
            result['errors'].append(f"Go parsing error: {str(e)}")

        return result

    def parse_python(self, content: str) -> Dict[str, Any]:
        """Parse Python file structure using AST"""
        result = {
            'functions': [],
            'classes': [],
            'imports': [],
            'errors': []
        }

        try:
            tree = ast.parse(content)

            # Parse imports
            for node in ast.walk(tree):
                if isinstance(node, ast.Import):
                    for alias in node.names:
                        result['imports'].append(alias.name)
                elif isinstance(node, ast.ImportFrom):
                    module = node.module or ''
                    for alias in node.names:
                        result['imports'].append(f"{module}.{alias.name}" if module else alias.name)

            # Parse functions and classes
            for node in ast.iter_child_nodes(tree):
                if isinstance(node, ast.FunctionDef):
                    # 获取函数结束行号
                    end_lineno = getattr(node, 'end_lineno', None)
                    if end_lineno is None:
                        # 如果没有end_lineno属性，尝试计算
                        end_lineno = node.lineno
                        # 遍历函数体找到最后一行
                        for child in ast.walk(node):
                            if hasattr(child, 'lineno'):
                                end_lineno = max(end_lineno, child.lineno)

                    result['functions'].append({
                        'name': node.name,
                        'type': 'function',
                        'lineno': node.lineno,
                        'end_lineno': end_lineno,
                        'col_offset': node.col_offset
                    })
                elif isinstance(node, ast.AsyncFunctionDef):
                    # 获取异步函数结束行号
                    end_lineno = getattr(node, 'end_lineno', None)
                    if end_lineno is None:
                        end_lineno = node.lineno
                        for child in ast.walk(node):
                            if hasattr(child, 'lineno'):
                                end_lineno = max(end_lineno, child.lineno)

                    result['functions'].append({
                        'name': node.name,
                        'type': 'async_function',
                        'lineno': node.lineno,
                        'end_lineno': end_lineno,
                        'col_offset': node.col_offset
                    })
                elif isinstance(node, ast.ClassDef):
                    # 获取类结束行号
                    end_lineno = getattr(node, 'end_lineno', None)
                    if end_lineno is None:
                        end_lineno = node.lineno
                        for child in ast.walk(node):
                            if hasattr(child, 'lineno'):
                                end_lineno = max(end_lineno, child.lineno)

                    result['classes'].append({
                        'name': node.name,
                        'type': 'class',
                        'lineno': node.lineno,
                        'end_lineno': end_lineno,
                        'col_offset': node.col_offset,
                        'methods': [method.name for method in node.body if isinstance(method, (ast.FunctionDef, ast.AsyncFunctionDef))]
                    })
        except SyntaxError as e:
            result['errors'].append(f"Python syntax error: {str(e)}")
        except Exception as e:
            result['errors'].append(f"Python parsing error: {str(e)}")
            result['errors'].append(traceback.format_exc())

        return result

    def parse_javascript(self, content: str) -> Dict[str, Any]:
        """Parse JavaScript file structure using regex"""
        result = {
            'functions': [],
            'classes': [],
            'imports': [],
            'errors': []
        }

        try:
            # Parse imports (ES6 modules)
            import_patterns = [
                r'import\s+(?:\*\s+as\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s+from\s+["\']([^"\']+)["\']',
                r'import\s+{([^}]+)}\s+from\s+["\']([^"\']+)["\']',
                r'import\s+["\']([^"\']+)["\']'
            ]

            for pattern in import_patterns:
                matches = re.findall(pattern, content)
                for match in matches:
                    if isinstance(match, tuple):
                        if len(match) == 2:
                            # import {x, y} from 'module' or import x from 'module'
                            if ',' in match[0]:
                                # Multiple imports: import {x, y} from 'module'
                                imports = match[0].split(',')
                                for imp in imports:
                                    imp = imp.strip()
                                    if imp:
                                        result['imports'].append(f"{imp} from {match[1]}")
                            else:
                                # Single import: import x from 'module'
                                result['imports'].append(f"{match[0]} from {match[1]}")
                        else:
                            # import 'module'
                            result['imports'].append(match[0])
                    else:
                        # import x from 'module' (non-tuple match)
                        result['imports'].append(f"{match}")

            # Parse functions
            func_patterns = [
                r'(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*\([^)]*\)',
                r'(?:export\s+)?(?:async\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=]+)?\s*=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>',
                r'(?:export\s+)?(?:async\s+)?let\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=]+)?\s*=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>',
                r'(?:export\s+)?(?:async\s+)?var\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=]+)?\s*=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>',
                r'(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*<[^>]*>\s*\([^)]*\)'  # Generic functions
            ]

            for pattern in func_patterns:
                matches = re.findall(pattern, content)
                for func_name in matches:
                    result['functions'].append({
                        'name': func_name,
                        'type': 'function'
                    })

            # Parse classes
            class_pattern = r'(?:export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)'
            classes = re.findall(class_pattern, content)
            for class_name in classes:
                result['classes'].append({
                    'name': class_name,
                    'type': 'class'
                })

        except Exception as e:
            result['errors'].append(f"JavaScript parsing error: {str(e)}")

        return result

    def parse_typescript(self, content: str) -> Dict[str, Any]:
        """Parse TypeScript file structure (extends JavaScript parsing)"""
        result = self.parse_javascript(content)
        result['language'] = 'typescript'

        try:
            # Additional TypeScript-specific parsing
            # Parse interfaces
            interface_pattern = r'(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]+)(?:\s*<[^>]*>)?\s*{'
            interfaces = re.findall(interface_pattern, content)
            if 'interfaces' not in result:
                result['interfaces'] = []
            for interface_name in interfaces:
                result['interfaces'].append({
                    'name': interface_name,
                    'type': 'interface'
                })

            # Parse types
            type_pattern = r'(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]+)(?:\s*<[^>]*>)?\s*='
            types = re.findall(type_pattern, content)
            if 'types' not in result:
                result['types'] = []
            for type_name in types:
                result['types'].append({
                    'name': type_name,
                    'type': 'type_alias'
                })

            # Parse enums
            enum_pattern = r'(?:export\s+)?enum\s+([A-Za-z_$][A-Za-z0-9_$]+)\s*{'
            enums = re.findall(enum_pattern, content)
            if 'enums' not in result:
                result['enums'] = []
            for enum_name in enums:
                result['enums'].append({
                    'name': enum_name,
                    'type': 'enum'
                })

        except Exception as e:
            result['errors'].append(f"TypeScript parsing error: {str(e)}")

        return result

    def parse_java(self, content: str) -> Dict[str, Any]:
        """Parse Java file structure"""
        result = {
            'classes': [],
            'methods': [],
            'imports': [],
            'errors': []
        }

        try:
            # Parse imports
            import_pattern = r'import\s+([\w.]+(?:\.[\w*]+)?)\s*;'
            imports = re.findall(import_pattern, content)
            result['imports'] = imports

            # Parse classes and interfaces
            class_pattern = r'(?:public\s+|private\s+|protected\s+|abstract\s+|final\s+)*(?:class|interface|enum)\s+([A-Za-z_$][A-Za-z0-9_$]*)'
            classes = re.findall(class_pattern, content)
            for class_name in classes:
                result['classes'].append({
                    'name': class_name,
                    'type': 'class'
                })

            # Parse methods
            method_pattern = r'(?:public\s+|private\s+|protected\s+|static\s+|final\s+|abstract\s+|synchronized\s+)*([A-Za-z_$<>\[\]\s]+)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^)]*\)'
            methods = re.findall(method_pattern, content)
            for return_type, method_name in methods:
                result['methods'].append({
                    'name': method_name.strip(),
                    'type': 'method',
                    'return_type': return_type.strip()
                })

        except Exception as e:
            result['errors'].append(f"Java parsing error: {str(e)}")

        return result

    def parse_c(self, content: str) -> Dict[str, Any]:
        """Parse C file structure"""
        result = {
            'functions': [],
            'includes': [],
            'structs': [],
            'errors': []
        }

        try:
            lines = content.split('\n')

            # Parse includes
            include_pattern = r'#include\s+[<"]([^>"]+)[>"]'
            for i, line in enumerate(lines):
                include_match = re.search(include_pattern, line)
                if include_match:
                    result['includes'].append({
                        'name': include_match.group(1),
                        'type': 'include',
                        'lineno': i + 1
                    })

            # Parse functions
            func_pattern = r'(?:[\w\s\*]+)\s+([\w]+)\s*\([^)]*\)\s*(?:\{[^}]*\})?'
            for i, line in enumerate(lines):
                func_match = re.search(func_pattern, line)
                if func_match:
                    # 查找函数体的结束
                    brace_count = line.count('{') - line.count('}')
                    if brace_count > 0:
                        # 函数体在同一行开始
                        for j in range(i + 1, len(lines)):
                            brace_count += lines[j].count('{') - lines[j].count('}')
                            if brace_count <= 0:
                                result['functions'].append({
                                    'name': func_match.group(1),
                                    'type': 'function',
                                    'lineno': i + 1,
                                    'end_lineno': j + 1
                                })
                                break
                    else:
                        # 函数体可能在后几行开始
                        for j in range(i + 1, min(i + 10, len(lines))):
                            if '{' in lines[j]:
                                # 找到函数体开始
                                brace_count = 1
                                for k in range(j + 1, len(lines)):
                                    brace_count += lines[k].count('{') - lines[k].count('}')
                                    if brace_count <= 0:
                                        result['functions'].append({
                                            'name': func_match.group(1),
                                            'type': 'function',
                                            'lineno': i + 1,
                                            'end_lineno': k + 1
                                        })
                                        break
                                break
                        else:
                            # 没有找到函数体，可能只有声明
                            result['functions'].append({
                                'name': func_match.group(1),
                                'type': 'function',
                                'lineno': i + 1
                            })

            # Parse structs
            struct_pattern = r'struct\s+([\w]+)\s*\{'
            for i, line in enumerate(lines):
                struct_match = re.search(struct_pattern, line)
                if struct_match:
                    # 查找结构体的结束
                    brace_count = line.count('{') - line.count('}')
                    if brace_count > 0:
                        # 结构体在同一行开始
                        for j in range(i + 1, len(lines)):
                            brace_count += lines[j].count('{') - lines[j].count('}')
                            if brace_count <= 0:
                                result['structs'].append({
                                    'name': struct_match.group(1),
                                    'type': 'struct',
                                    'lineno': i + 1,
                                    'end_lineno': j + 1
                                })
                                break
                    else:
                        # 结构体可能在后几行开始
                        for j in range(i + 1, min(i + 10, len(lines))):
                            if '{' in lines[j]:
                                # 找到结构体开始
                                brace_count = 1
                                for k in range(j + 1, len(lines)):
                                    brace_count += lines[k].count('{') - lines[k].count('}')
                                    if brace_count <= 0:
                                        result['structs'].append({
                                            'name': struct_match.group(1),
                                            'type': 'struct',
                                            'lineno': i + 1,
                                            'end_lineno': k + 1
                                        })
                                        break
                                break
                        else:
                            # 没有找到结构体定义
                            result['structs'].append({
                                'name': struct_match.group(1),
                                'type': 'struct',
                                'lineno': i + 1
                            })

        except Exception as e:
            result['errors'].append(f"C parsing error: {str(e)}")

        return result

    def parse_cpp(self, content: str) -> Dict[str, Any]:
        """Parse C++ file structure (extends C parsing)"""
        result = self.parse_c(content)
        result['language'] = 'cpp'

        try:
            lines = content.split('\n')

            # Parse classes
            class_pattern = r'class\s+([\w]+)'
            for i, line in enumerate(lines):
                class_match = re.search(class_pattern, line)
                if class_match:
                    # 查找类的结束
                    brace_count = line.count('{') - line.count('}')
                    if brace_count > 0:
                        # 类在同一行开始
                        for j in range(i + 1, len(lines)):
                            brace_count += lines[j].count('{') - lines[j].count('}')
                            if brace_count <= 0:
                                result.setdefault('classes', []).append({
                                    'name': class_match.group(1),
                                    'type': 'class',
                                    'lineno': i + 1,
                                    'end_lineno': j + 1
                                })
                                break
                    else:
                        # 类可能在后几行开始
                        for j in range(i + 1, min(i + 10, len(lines))):
                            if '{' in lines[j]:
                                # 找到类开始
                                brace_count = 1
                                for k in range(j + 1, len(lines)):
                                    brace_count += lines[k].count('{') - lines[k].count('}')
                                    if brace_count <= 0:
                                        result.setdefault('classes', []).append({
                                            'name': class_match.group(1),
                                            'type': 'class',
                                            'lineno': i + 1,
                                            'end_lineno': k + 1
                                        })
                                        break
                                break
                        else:
                            # 没有找到类定义
                            result.setdefault('classes', []).append({
                                'name': class_match.group(1),
                                'type': 'class',
                                'lineno': i + 1
                            })

            # Parse namespaces
            namespace_pattern = r'namespace\s+([\w]+)'
            for i, line in enumerate(lines):
                namespace_match = re.search(namespace_pattern, line)
                if namespace_match:
                    # 查找命名空间的结束
                    brace_count = line.count('{') - line.count('}')
                    if brace_count > 0:
                        # 命名空间在同一行开始
                        for j in range(i + 1, len(lines)):
                            brace_count += lines[j].count('{') - lines[j].count('}')
                            if brace_count <= 0:
                                result.setdefault('namespaces', []).append({
                                    'name': namespace_match.group(1),
                                    'type': 'namespace',
                                    'lineno': i + 1,
                                    'end_lineno': j + 1
                                })
                                break
                    else:
                        # 命名空间可能在后几行开始
                        for j in range(i + 1, min(i + 10, len(lines))):
                            if '{' in lines[j]:
                                # 找到命名空间开始
                                brace_count = 1
                                for k in range(j + 1, len(lines)):
                                    brace_count += lines[k].count('{') - lines[k].count('}')
                                    if brace_count <= 0:
                                        result.setdefault('namespaces', []).append({
                                            'name': namespace_match.group(1),
                                            'type': 'namespace',
                                            'lineno': i + 1,
                                            'end_lineno': k + 1
                                        })
                                        break
                                break
                        else:
                            # 没有找到命名空间定义
                            result.setdefault('namespaces', []).append({
                                'name': namespace_match.group(1),
                                'type': 'namespace',
                                'lineno': i + 1
                            })

        except Exception as e:
            result['errors'].append(f"C++ parsing error: {str(e)}")
        return result

    def parse_markdown(self, content: str) -> Dict[str, Any]:
        """Parse Markdown file structure"""
        result = {
            'sections': [],
            'headings': [],
            'code_blocks': [],
            'lists': [],
            'links': [],
            'errors': []
        }

        try:
            lines = content.split('\n')
            in_code_block = False
            code_block_start = 0
            current_code_block_language = ''

            for i, line in enumerate(lines):
                # 解析标题
                heading_match = re.match(r'^(#{1,6})\s+(.+)$', line)
                if heading_match:
                    level = len(heading_match.group(1))
                    text = heading_match.group(2).strip()
                    result['headings'].append({
                        'name': text,
                        'type': f'heading_{level}',
                        'lineno': i + 1
                    })

                # 解析代码块
                if line.strip().startswith('```'):
                    if not in_code_block:
                        # 代码块开始
                        in_code_block = True
                        code_block_start = i + 1
                        current_code_block_language = line.strip()[3:] if len(line.strip()) > 3 else ''
                    else:
                        # 代码块结束
                        in_code_block = False
                        result['code_blocks'].append({
                            'name': f'code_block_{code_block_start}',
                            'type': 'code_block',
                            'lineno': code_block_start,
                            'end_lineno': i + 1,
                            'language': current_code_block_language
                        })

                # 解析列表项
                list_match = re.match(r'^(\s*)[-*+]\s+(.+)$', line)
                if list_match:
                    indent = len(list_match.group(1))
                    text = list_match.group(2).strip()
                    result['lists'].append({
                        'name': text,
                        'type': 'list_item',
                        'lineno': i + 1,
                        'indent': indent
                    })

                # 解析链接
                link_pattern = r'\[([^\]]+)\]\(([^)]+)\)'
                for link_match in re.finditer(link_pattern, line):
                    result['links'].append({
                        'name': link_match.group(1),
                        'type': 'link',
                        'lineno': i + 1,
                        'url': link_match.group(2)
                    })

            # 如果文件以未关闭的代码块结束
            if in_code_block:
                result['code_blocks'].append({
                    'name': f'code_block_{code_block_start}',
                    'type': 'code_block',
                    'lineno': code_block_start,
                    'end_lineno': len(lines),
                    'language': current_code_block_language
                })

        except Exception as e:
            result['errors'].append(f"Markdown parsing error: {str(e)}")

        return result

    def parse_org(self, content: str) -> Dict[str, Any]:
        """Parse Org-mode file structure"""
        result = {
            'headings': [],
            'code_blocks': [],
            'tables': [],
            'lists': [],
            'errors': []
        }

        try:
            lines = content.split('\n')
            in_code_block = False
            code_block_start = 0

            for i, line in enumerate(lines):
                # 解析标题
                heading_match = re.match(r'^(\*+)\s+(.+)$', line)
                if heading_match:
                    level = len(heading_match.group(1))
                    text = heading_match.group(2).strip()
                    result['headings'].append({
                        'name': text,
                        'type': f'heading_{level}',
                        'lineno': i + 1
                    })

                # 解析代码块
                if line.strip().startswith('#+BEGIN_SRC'):
                    in_code_block = True
                    code_block_start = i + 1
                elif line.strip().startswith('#+END_SRC'):
                    in_code_block = False
                    result['code_blocks'].append({
                        'name': f'code_block_{code_block_start}',
                        'type': 'code_block',
                        'lineno': code_block_start,
                        'end_lineno': i + 1
                    })

                # 解析表格
                if line.strip().startswith('|'):
                    result['tables'].append({
                        'name': f'table_{i+1}',
                        'type': 'table',
                        'lineno': i + 1
                    })

                # 解析列表项
                list_match = re.match(r'^(\s*)[-+]\s+(.+)$', line)
                if list_match:
                    indent = len(list_match.group(1))
                    text = list_match.group(2).strip()
                    result['lists'].append({
                        'name': text,
                        'type': 'list_item',
                        'lineno': i + 1,
                        'indent': indent
                    })

            # 如果文件以未关闭的代码块结束
            if in_code_block:
                result['code_blocks'].append({
                    'name': f'code_block_{code_block_start}',
                    'type': 'code_block',
                    'lineno': code_block_start,
                    'end_lineno': len(lines)
                })

        except Exception as e:
            result['errors'].append(f"Org-mode parsing error: {str(e)}")

        return result
        return result

    def parse_elisp(self, content: str) -> Dict[str, Any]:
        """Parse Emacs Lisp file structure"""
        result = {
            'functions': [],
            'variables': [],
            'macros': [],
            'custom_variables': [],
            'provides': [],
            'errors': []
        }

        try:
            lines = content.split('\n')

            for i, line in enumerate(lines):
                line_num = i + 1
                stripped_line = line.strip()

                # 解析函数定义 (defun)
                defun_match = re.match(r'\(defun\s+([^\s\(]+)', stripped_line)
                if defun_match:
                    func_name = defun_match.group(1)
                    # 查找函数结束
                    paren_count = stripped_line.count('(') - stripped_line.count(')')
                    end_line = line_num

                    if paren_count > 0:
                        # 函数定义跨越多行
                        for j in range(i + 1, len(lines)):
                            paren_count += lines[j].count('(') - lines[j].count(')')
                            if paren_count <= 0:
                                end_line = j + 1
                                break

                    result['functions'].append({
                        'name': func_name,
                        'type': 'function',
                        'lineno': line_num,
                        'end_lineno': end_line
                    })

                # 解析变量定义 (defvar)
                defvar_match = re.match(r'\(defvar\s+([^\s\(]+)', stripped_line)
                if defvar_match:
                    var_name = defvar_match.group(1)
                    result['variables'].append({
                        'name': var_name,
                        'type': 'variable',
                        'lineno': line_num
                    })

                # 解析自定义变量 (defcustom)
                defcustom_match = re.match(r'\(defcustom\s+([^\s\(]+)', stripped_line)
                if defcustom_match:
                    var_name = defcustom_match.group(1)
                    result['custom_variables'].append({
                        'name': var_name,
                        'type': 'custom_variable',
                        'lineno': line_num
                    })

                # 解析宏定义 (defmacro)
                defmacro_match = re.match(r'\(defmacro\s+([^\s\(]+)', stripped_line)
                if defmacro_match:
                    macro_name = defmacro_match.group(1)
                    result['macros'].append({
                        'name': macro_name,
                        'type': 'macro',
                        'lineno': line_num
                    })

                # 解析提供模块 (provide)
                provide_match = re.match(r'\(provide\s+\'([^\s\)]+)', stripped_line)
                if provide_match:
                    module_name = provide_match.group(1)
                    result['provides'].append({
                        'name': module_name,
                        'type': 'provide',
                        'lineno': line_num
                    })

                # 解析注释
                if stripped_line.startswith(';;;'):
                    # 文件头注释
                    result.setdefault('comments', []).append({
                        'name': stripped_line[3:].strip(),
                        'type': 'file_header_comment',
                        'lineno': line_num
                    })
                elif stripped_line.startswith(';;'):
                    # 节注释
                    result.setdefault('comments', []).append({
                        'name': stripped_line[2:].strip(),
                        'type': 'section_comment',
                        'lineno': line_num
                    })
                elif stripped_line.startswith(';'):
                    # 行内注释
                    result.setdefault('comments', []).append({
                        'name': stripped_line[1:].strip(),
                        'type': 'inline_comment',
                        'lineno': line_num
                    })

        except Exception as e:
            result['errors'].append(f"Emacs Lisp parsing error: {str(e)}")

        return result

    def parse_makefile(self, content: str) -> Dict[str, Any]:
        """Parse Makefile structure"""
        result = {
            'targets': [],
            'variables': [],
            'phony_targets': [],
            'functions': [],
            'conditionals': [],
            'errors': []
        }

        try:
            lines = content.split('\n')

            for i, line in enumerate(lines):
                line_num = i + 1
                stripped_line = line.strip()

                # 跳过空行和注释
                if not stripped_line or stripped_line.startswith('#'):
                    continue

                # 解析变量定义 (VAR = value)
                var_match = re.match(r'^([A-Za-z_][A-Za-z0-9_]*)\s*[:?+]?=\s*(.+)$', stripped_line)
                if var_match:
                    var_name = var_match.group(1)
                    var_value = var_match.group(2).strip()
                    result['variables'].append({
                        'name': var_name,
                        'type': 'variable',
                        'value': var_value,
                        'lineno': line_num
                    })
                    continue

                # 解析目标定义 (target: dependencies)
                target_match = re.match(r'^([^:#=\s]+)\s*:(.*)$', stripped_line)
                if target_match:
                    target_name = target_match.group(1).strip()
                    dependencies = target_match.group(2).strip()

                    # 检查是否是伪目标
                    if target_name == '.PHONY':
                        # 解析伪目标列表
                        phony_targets = [t.strip() for t in dependencies.split() if t.strip()]
                        for phony_target in phony_targets:
                            result['phony_targets'].append({
                                'name': phony_target,
                                'type': 'phony_target',
                                'lineno': line_num
                            })
                    else:
                        result['targets'].append({
                            'name': target_name,
                            'type': 'target',
                            'dependencies': dependencies,
                            'lineno': line_num
                        })
                    continue

                # 解析函数调用 ($(shell command) 或 $(function args))
                function_match = re.search(r'\$\(([^)]+)\)', stripped_line)
                if function_match:
                    func_call = function_match.group(1)
                    # 检查是否是shell函数
                    if func_call.startswith('shell '):
                        result['functions'].append({
                            'name': 'shell',
                            'type': 'shell_function',
                            'args': func_call[6:].strip(),
                            'lineno': line_num
                        })
                    else:
                        # 其他函数
                        result['functions'].append({
                            'name': func_call.split()[0] if ' ' in func_call else func_call,
                            'type': 'make_function',
                            'args': func_call,
                            'lineno': line_num
                        })

                # 解析条件语句 (ifeq, ifneq, ifdef, ifndef)
                conditional_patterns = [
                    (r'^ifeq\s+\((.+)\)', 'ifeq'),
                    (r'^ifneq\s+\((.+)\)', 'ifneq'),
                    (r'^ifdef\s+([^\s]+)', 'ifdef'),
                    (r'^ifndef\s+([^\s]+)', 'ifndef'),
                    (r'^else', 'else'),
                    (r'^endif', 'endif')
                ]

                for pattern, cond_type in conditional_patterns:
                    cond_match = re.match(pattern, stripped_line)
                    if cond_match:
                        condition = cond_match.group(1) if cond_match.groups() else ''
                        result['conditionals'].append({
                            'name': cond_type,
                            'type': 'conditional',
                            'condition': condition,
                            'lineno': line_num
                        })
                        break

                # 解析包含 (include)
                include_match = re.match(r'^include\s+(.+)$', stripped_line)
                if include_match:
                    included_file = include_match.group(1).strip()
                    result.setdefault('includes', []).append({
                        'name': included_file,
                        'type': 'include',
                        'lineno': line_num
                    })

        except Exception as e:
            result['errors'].append(f"Makefile parsing error: {str(e)}")

        return result

    def parse_cmake(self, content: str) -> Dict[str, Any]:
        """Parse CMake file structure"""
        result = {
            'commands': [],
            'variables': [],
            'targets': [],
            'tests': [],
            'installs': [],
            'errors': []
        }

        try:
            lines = content.split('\n')

            for i, line in enumerate(lines):
                line_num = i + 1
                stripped_line = line.strip()

                # 跳过空行和注释
                if not stripped_line or stripped_line.startswith('#'):
                    continue

                # 解析CMake命令 (command(arg1 arg2 ...))
                # 匹配命令名和参数
                cmd_match = re.match(r'^([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)', stripped_line)
                if cmd_match:
                    cmd_name = cmd_match.group(1)
                    cmd_args = cmd_match.group(2).strip()

                    # 根据命令类型分类
                    cmd_info = {
                        'name': cmd_name,
                        'type': 'command',
                        'args': cmd_args,
                        'lineno': line_num
                    }

                    # 特殊处理一些常见命令
                    if cmd_name == 'cmake_minimum_required':
                        cmd_info['type'] = 'minimum_version'
                    elif cmd_name == 'project':
                        cmd_info['type'] = 'project_declaration'
                    elif cmd_name == 'set':
                        # 解析变量设置
                        var_match = re.match(r'([A-Za-z_][A-Za-z0-9_]*)\s+(.+)', cmd_args)
                        if var_match:
                            var_name = var_match.group(1)
                            var_value = var_match.group(2).strip()
                            result['variables'].append({
                                'name': var_name,
                                'type': 'variable',
                                'value': var_value,
                                'lineno': line_num
                            })
                    elif cmd_name == 'add_executable':
                        # 解析可执行文件目标
                        target_match = re.match(r'([A-Za-z_][A-Za-z0-9_]*)\s+(.+)', cmd_args)
                        if target_match:
                            target_name = target_match.group(1)
                            sources = target_match.group(2).strip()
                            result['targets'].append({
                                'name': target_name,
                                'type': 'executable',
                                'sources': sources,
                                'lineno': line_num
                            })
                    elif cmd_name == 'add_library':
                        # 解析库目标
                        lib_match = re.match(r'([A-Za-z_][A-Za-z0-9_]*)\s+(STATIC|SHARED|MODULE)?\s*(.+)', cmd_args)
                        if lib_match:
                            lib_name = lib_match.group(1)
                            lib_type = lib_match.group(2) or 'STATIC'
                            sources = lib_match.group(3).strip()
                            result['targets'].append({
                                'name': lib_name,
                                'type': 'library',
                                'library_type': lib_type,
                                'sources': sources,
                                'lineno': line_num
                            })
                    elif cmd_name == 'target_link_libraries':
                        # 解析库链接
                        link_match = re.match(r'([A-Za-z_][A-Za-z0-9_]*)\s+(.+)', cmd_args)
                        if link_match:
                            target_name = link_match.group(1)
                            libraries = link_match.group(2).strip()
                            cmd_info['target'] = target_name
                            cmd_info['libraries'] = libraries
                    elif cmd_name == 'add_test':
                        # 解析测试
                        test_match = re.match(r'NAME\s+([A-Za-z_][A-Za-z0-9_]*)\s+COMMAND\s+(.+)', cmd_args)
                        if test_match:
                            test_name = test_match.group(1)
                            test_command = test_match.group(2).strip()
                            result['tests'].append({
                                'name': test_name,
                                'type': 'test',
                                'command': test_command,
                                'lineno': line_num
                            })
                    elif cmd_name == 'install':
                        # 解析安装命令
                        install_match = re.match(r'TARGETS\s+([A-Za-z_][A-Za-z0-9_]+(?:\s+[A-Za-z_][A-Za-z0-9_]+)*)', cmd_args)
                        if install_match:
                            targets = install_match.group(1).strip()
                            result['installs'].append({
                                'name': 'install_targets',
                                'type': 'install',
                                'targets': targets,
                                'lineno': line_num
                            })
                    elif cmd_name == 'include_directories':
                        cmd_info['type'] = 'include_directories'
                    elif cmd_name == 'enable_testing':
                        cmd_info['type'] = 'enable_testing'
                    elif cmd_name == 'add_custom_command':
                        cmd_info['type'] = 'custom_command'

                    result['commands'].append(cmd_info)

                # 解析变量引用 (${VAR})
                var_ref_match = re.search(r'\$\{([^}]+)\}', stripped_line)
                if var_ref_match:
                    var_name = var_ref_match.group(1)
                    result.setdefault('variable_references', []).append({
                        'name': var_name,
                        'type': 'variable_reference',
                        'lineno': line_num
                    })

        except Exception as e:
            result['errors'].append(f"CMake parsing error: {str(e)}")

        return result

    def parse_vimscript(self, content: str) -> Dict[str, Any]:
        """Parse Vimscript file structure"""
        result = {
            'functions': [],
            'commands': [],
            'variables': [],
            'mappings': [],
            'autocommands': [],
            'augroups': [],
            'errors': []
        }

        try:
            lines = content.split('\n')
            in_function = False
            current_function = None
            function_start_line = 0

            for i, line in enumerate(lines):
                line_num = i + 1
                stripped_line = line.strip()

                # Skip empty lines and comments
                if not stripped_line or stripped_line.startswith('"'):
                    continue

                # Parse function definitions
                # Match: function! FunctionName() or function FunctionName()
                func_match = re.match(r'^\s*(?:function!?|def)\s+([A-Za-z_][A-Za-z0-9_:]*)\s*\(', stripped_line)
                if func_match and not in_function:
                    func_name = func_match.group(1)
                    # Check if it's a script-local function
                    func_type = 'script_function' if func_name.startswith('s:') else 'function'

                    # Vimscript functions end with 'endfunction'
                    in_function = True
                    current_function = func_name
                    current_function_type = func_type
                    function_start_line = line_num

                # Check for endfunction
                if in_function and stripped_line == 'endfunction':
                    result['functions'].append({
                        'name': current_function,
                        'type': current_function_type,
                        'lineno': function_start_line,
                        'end_lineno': line_num
                    })
                    in_function = False
                    current_function = None
                    current_function_type = None

                # Parse command definitions
                # Match: command! CommandName
                cmd_match = re.match(r'^\s*command!\s+([A-Za-z_][A-Za-z0-9_]*)', stripped_line)
                if cmd_match:
                    cmd_name = cmd_match.group(1)
                    result['commands'].append({
                        'name': cmd_name,
                        'type': 'command',
                        'lineno': line_num
                    })

                # Parse variable definitions
                # Match: let var = value or let g:var = value
                var_match = re.match(r'^\s*let\s+([gs]:)?([A-Za-z_][A-Za-z0-9_]*)\s*=', stripped_line)
                if var_match:
                    scope = var_match.group(1) or ''
                    var_name = var_match.group(2)
                    var_type = 'variable'
                    if scope == 'g:':
                        var_type = 'global_variable'
                    elif scope == 's:':
                        var_type = 'script_variable'

                    result['variables'].append({
                        'name': var_name,
                        'type': var_type,
                        'lineno': line_num
                    })

                # Parse mappings
                # Match: nnoremap, inoremap, vnoremap, etc.
                map_patterns = [
                    (r'^\s*(nnoremap|inoremap|vnoremap|xnoremap|snoremap|onoremap|tnoremap|noremap|imap|vmap|xmap|smap|omap|tmap|map)\s+', 'mapping'),
                    (r'^\s*(nunmap|iunmap|vunmap|xunmap|sunmap|ounmap|tunmap|unmap)\s+', 'unmap')
                ]

                for pattern, map_type in map_patterns:
                    map_match = re.match(pattern, stripped_line)
                    if map_match:
                        map_cmd = map_match.group(1)
                        # Extract the mapping (skip the command part)
                        mapping_part = stripped_line[len(map_cmd):].strip()
                        # Split into lhs and rhs if possible
                        parts = mapping_part.split(None, 1)
                        if len(parts) >= 1:
                            lhs = parts[0]
                            rhs = parts[1] if len(parts) > 1 else ''
                            result['mappings'].append({
                                'name': f'{map_cmd} {lhs}',
                                'type': map_type,
                                'command': map_cmd,
                                'lhs': lhs,
                                'rhs': rhs,
                                'lineno': line_num
                            })
                        break

                # Parse autocommand groups
                augroup_match = re.match(r'^\s*augroup\s+([A-Za-z_][A-Za-z0-9_]*)', stripped_line)
                if augroup_match:
                    augroup_name = augroup_match.group(1)
                    result['augroups'].append({
                        'name': augroup_name,
                        'type': 'augroup',
                        'lineno': line_num
                    })

                # Parse autocommands
                autocmd_match = re.match(r'^\s*autocmd!\s*', stripped_line)
                if autocmd_match:
                    # autocmd! (clear autocommands)
                    result['autocommands'].append({
                        'name': 'autocmd_clear',
                        'type': 'autocmd_clear',
                        'lineno': line_num
                    })
                else:
                    autocmd_match = re.match(r'^\s*autocmd\s+', stripped_line)
                    if autocmd_match:
                        # Parse autocmd with event and pattern
                        autocmd_parts = stripped_line[7:].strip().split(None, 2)
                        if len(autocmd_parts) >= 2:
                            event = autocmd_parts[0]
                            pattern = autocmd_parts[1]
                            command = autocmd_parts[2] if len(autocmd_parts) > 2 else ''
                            result['autocommands'].append({
                                'name': f'autocmd {event} {pattern}',
                                'type': 'autocmd',
                                'event': event,
                                'pattern': pattern,
                                'command': command,
                                'lineno': line_num
                            })

            # Handle function that ends at EOF (missing endfunction)
            if in_function and current_function:
                result['functions'].append({
                    'name': current_function,
                    'type': 'function',
                    'lineno': function_start_line,
                    'end_lineno': len(lines)
                })

        except Exception as e:
            result['errors'].append(f"Vimscript parsing error: {str(e)}")

        return result

    def _check_dependencies(self) -> bool:
        """Check required standard library dependencies using find_spec."""
        required_deps = ['json', 're', 'ast', 'typing', 'traceback']
        for dep in required_deps:
            if importlib.util.find_spec(dep) is None:
                return False
        return True


    def _get_enhanced_capabilities(self) -> List[str]:
        """Get enhanced parsing capabilities based on available optional dependencies."""
        capabilities = []

        if importlib.util.find_spec('astroid') is not None:
            capabilities.append('python_enhanced')

        if importlib.util.find_spec('javalang') is not None:
            capabilities.append('java_enhanced')

        if importlib.util.find_spec('pycparser') is not None:
            capabilities.append('c_enhanced')
            capabilities.append('cpp_enhanced')

        return capabilities

    def get_dependency_info(self) -> Dict[str, Any]:
        """Get information about dependencies and capabilities"""
        return {
            'dependencies_ok': self.deps_ok,
            'enhanced_capabilities': self.enhanced_capabilities,
            'python_version': sys.version,
            'supported_languages': list(self.language_parsers.keys())
        }


def main():
    """Main entry point for the script"""
    try:
        # Read input from stdin
        input_data = sys.stdin.read().strip()

        if not input_data:
            print(json.dumps({
                'error': 'No input provided',
                'usage': 'echo \'{"content": "code", "language": "python"}\' | python3 parse.py'
            }, indent=2))
            sys.exit(1)

        # Parse input JSON
        try:
            data = json.loads(input_data)
        except json.JSONDecodeError as e:
            print(json.dumps({
                'error': f'Invalid JSON input: {str(e)}',
                'input': input_data[:100] + '...' if len(input_data) > 100 else input_data
            }, indent=2))
            sys.exit(1)

        # Check for dependency check request
        if data.get('action') == 'check_deps':
            parser = FileStructureParser()
            result = parser.get_dependency_info()
            print(json.dumps(result, indent=2))
            sys.exit(0)

        # Validate input for parsing
        if 'content' not in data:
            print(json.dumps({
                'error': 'Missing required field: content'
            }, indent=2))
            sys.exit(1)

        if 'language' not in data:
            print(json.dumps({
                'error': 'Missing required field: language'
            }, indent=2))
            sys.exit(1)

        # Create parser and check dependencies
        parser = FileStructureParser()

        # Add dependency info to result
        result = parser.parse(data['content'], data['language'])
        result['dependency_info'] = parser.get_dependency_info()

        # Output result as JSON
        print(json.dumps(result, indent=2))

    except Exception as e:
        print(json.dumps({
            'error': f'Unexpected error: {str(e)}',
            'traceback': traceback.format_exc()
        }, indent=2))
        sys.exit(1)


if __name__ == '__main__':
    main()