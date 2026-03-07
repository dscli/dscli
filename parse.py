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
from typing import Dict, List, Any, Optional
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
        }
        
        # Check dependencies
        self.deps_ok = self._check_dependencies()
        self.enhanced_capabilities = self._get_enhanced_capabilities()
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
            # Parse imports
            import_pattern = r'import\s*(?:\(([\s\S]*?)\)|"([^"]+)")'
            imports = re.findall(import_pattern, content)
            for imp in imports:
                if imp[0]:  # Multi-line imports in parentheses
                    lines = imp[0].strip().split('\n')
                    for line in lines:
                        line = line.strip()
                        if line and not line.startswith('//'):
                            if line.startswith('"'):
                                result['imports'].append(line.strip('"'))
                elif imp[1]:  # Single import
                    result['imports'].append(imp[1].strip('"'))
            
            # Parse functions
            func_pattern = r'func\s+(?:\([^)]+\)\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)\s*(?:\([^)]*\))?\s*(?:\{[^}]*\})?'
            functions = re.findall(func_pattern, content)
            for func_name in functions:
                result['functions'].append({
                    'name': func_name,
                    'type': 'function'
                })
            
            # Parse structs
            struct_pattern = r'type\s+([A-Za-z_][A-Za-z0-9_]*)\s+struct\s*\{'
            structs = re.findall(struct_pattern, content)
            for struct_name in structs:
                result['structs'].append({
                    'name': struct_name,
                    'type': 'struct'
                })
            
            # Parse interfaces
            interface_pattern = r'type\s+([A-Za-z_][A-Za-z0-9_]*)\s+interface\s*\{'
            interfaces = re.findall(interface_pattern, content)
            for interface_name in interfaces:
                result['interfaces'].append({
                    'name': interface_name,
                    'type': 'interface'
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
                    result['functions'].append({
                        'name': node.name,
                        'type': 'function',
                        'lineno': node.lineno,
                        'col_offset': node.col_offset
                    })
                elif isinstance(node, ast.AsyncFunctionDef):
                    result['functions'].append({
                        'name': node.name,
                        'type': 'async_function',
                        'lineno': node.lineno,
                        'col_offset': node.col_offset
                    })
                elif isinstance(node, ast.ClassDef):
                    result['classes'].append({
                        'name': node.name,
                        'type': 'class',
                        'lineno': node.lineno,
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
                            # import {x, y} from 'module'
                            imports = match[0].split(',')
                            for imp in imports:
                                imp = imp.strip()
                                if imp:
                                    result['imports'].append(f"{imp} from {match[1]}")
                        else:
                            # import 'module'
                            result['imports'].append(match)
                    else:
                        # import x from 'module'
                        result['imports'].append(f"{match}")
            
            # Parse functions
            func_patterns = [
                r'(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^)]*\)',
                r'(?:export\s+)?(?:async\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>',
                r'(?:export\s+)?(?:async\s+)?let\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>',
                r'(?:export\s+)?(?:async\s+)?var\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>'
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
            interface_pattern = r'(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)'
            interfaces = re.findall(interface_pattern, content)
            if 'interfaces' not in result:
                result['interfaces'] = []
            for interface_name in interfaces:
                result['interfaces'].append({
                    'name': interface_name,
                    'type': 'interface'
                })
            
            # Parse types
            type_pattern = r'(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*='
            types = re.findall(type_pattern, content)
            if 'types' not in result:
                result['types'] = []
            for type_name in types:
                result['types'].append({
                    'name': type_name,
                    'type': 'type_alias'
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
            # Parse includes
            include_pattern = r'#include\s+[<"]([^>"]+)[>"]'
            includes = re.findall(include_pattern, content)
            result['includes'] = includes
            
            # Parse functions
            func_pattern = r'(?:[\w\s\*]+)\s+([\w]+)\s*\([^)]*\)\s*(?:\{[^}]*\})?'
            functions = re.findall(func_pattern, content)
            for func_name in functions:
                result['functions'].append({
                    'name': func_name,
                    'type': 'function'
                })
            
            # Parse structs
            struct_pattern = r'struct\s+([\w]+)\s*\{'
            structs = re.findall(struct_pattern, content)
            for struct_name in structs:
                result['structs'].append({
                    'name': struct_name,
                    'type': 'struct'
                })
                
        except Exception as e:
            result['errors'].append(f"C parsing error: {str(e)}")
        
        return result
    
    def parse_cpp(self, content: str) -> Dict[str, Any]:
        """Parse C++ file structure (extends C parsing)"""
        result = self.parse_c(content)
        result['language'] = 'cpp'
        
        try:
            # Additional C++-specific parsing
            # Parse classes
            class_pattern = r'class\s+([\w]+)'
            classes = re.findall(class_pattern, content)
            if 'classes' not in result:
                result['classes'] = []
            for class_name in classes:
                result['classes'].append({
                    'name': class_name,
                    'type': 'class'
                })
            
            # Parse namespaces
            namespace_pattern = r'namespace\s+([\w]+)'
            namespaces = re.findall(namespace_pattern, content)
            if 'namespaces' not in result:
                result['namespaces'] = []
            for namespace_name in namespaces:
                result['namespaces'].append({
                    'name': namespace_name,
                    'type': 'namespace'
                })
                
        except Exception as e:
            result['errors'].append(f"C++ parsing error: {str(e)}")
        
        return result
    
    def parse(self, content: str, language: str) -> Dict[str, Any]:
        """Main parsing method"""
        language = language.lower()
        
        if language not in self.language_parsers:
            return {
                'error': f"Unsupported language: {language}",
                'supported_languages': list(self.language_parsers.keys())
            }
        
        try:
            result = self.language_parsers[language](content)
            result['language'] = language
            result['success'] = True
            return result
        except Exception as e:
            return {
                'language': language,
                'success': False,
                'error': str(e),
                'traceback': traceback.format_exc()
            }
    
    def _check_dependencies(self) -> bool:
        """Check if required dependencies are available"""
        required_deps = ['json', 're', 'ast', 'typing', 'traceback', 'importlib.util']
        
        for dep in required_deps:
            try:
                if dep == 'importlib.util':
                    import importlib.util
                elif dep == 'json':
                    import json
                elif dep == 're':
                    import re
                elif dep == 'ast':
                    import ast
                elif dep == 'typing':
                    from typing import Dict, List, Any, Optional
                elif dep == 'traceback':
                    import traceback
            except ImportError:
                return False
        
        return True
    
    def _get_enhanced_capabilities(self) -> List[str]:
        """Get enhanced parsing capabilities based on available optional dependencies"""
        capabilities = []
        
        # Check for astroid (enhanced Python parsing)
        try:
            import astroid
            capabilities.append('python_enhanced')
        except ImportError:
            pass
        
        # Check for javalang (enhanced Java parsing)
        try:
            import javalang
            capabilities.append('java_enhanced')
        except ImportError:
            pass
        
        # Check for pycparser (enhanced C/C++ parsing)
        try:
            import pycparser
            capabilities.append('c_enhanced')
            capabilities.append('cpp_enhanced')
        except ImportError:
            pass
        
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
        sys.exit(0)
        print(json.dumps(result, indent=2))
        
    except Exception as e:
        print(json.dumps({
            'error': f'Unexpected error: {str(e)}',
            'traceback': traceback.format_exc()
        }, indent=2))
        sys.exit(1)


if __name__ == '__main__':
    main()
