#!/usr/bin/env python3
"""
Dependency Checker for dscli Python Parser

This module provides dependency checking capabilities to ensure
required Python packages are available before parsing.
"""

import sys
import json
import importlib
import importlib.util
import subprocess
import pkg_resources
from typing import Dict, List, Optional, Tuple, Any
import traceback


class DependencyChecker:
    """Check and manage Python dependencies for the parser"""
    
    # Required dependencies for core functionality
    REQUIRED_DEPS = {
        'json': 'json',  # Built-in
        're': 're',      # Built-in
        'ast': 'ast',    # Built-in
        'typing': 'typing',  # Built-in
        'traceback': 'traceback',  # Built-in
    }
    
    # Optional dependencies for enhanced parsing
    OPTIONAL_DEPS = {
        'astroid': {
            'package': 'astroid',
            'min_version': '3.0.0',
            'purpose': 'Enhanced Python AST parsing and type inference',
            'required_for': ['python_enhanced']
        },
        'javalang': {
            'package': 'javalang',
            'min_version': '0.13.0',
            'purpose': 'Java language parsing',
            'required_for': ['java_enhanced']
        },
        'pycparser': {
            'package': 'pycparser',
            'min_version': '2.21',
            'purpose': 'C/C++ parsing',
            'required_for': ['c_enhanced', 'cpp_enhanced']
        }
    }
    
    def __init__(self, verbose: bool = False):
        self.verbose = verbose
        self.available_deps = {}
        self.missing_deps = []
        self.optional_deps_status = {}
        
    def check_required_deps(self) -> bool:
        """Check if all required dependencies are available"""
        all_available = True
        
        for dep_name, module_name in self.REQUIRED_DEPS.items():
            try:
                spec = importlib.util.find_spec(module_name)
                if spec is None:
                    raise ImportError(f"Module {module_name} not found")
                
                self.available_deps[dep_name] = {
                    'module': module_name,
                    'version': self._get_module_version(module_name),
                    'available': True
                }
                
                if self.verbose:
                    print(f"✓ Required dependency '{dep_name}' ({module_name}) is available", 
                          file=sys.stderr)
                    
            except ImportError as e:
                self.missing_deps.append({
                    'name': dep_name,
                    'module': module_name,
                    'error': str(e),
                    'required': True
                })
                all_available = False
                
                if self.verbose:
                    print(f"✗ Required dependency '{dep_name}' ({module_name}) is missing: {e}", 
                          file=sys.stderr)
        
        return all_available
    
    def check_optional_deps(self) -> Dict[str, Any]:
        """Check optional dependencies and return their status"""
        for dep_name, dep_info in self.OPTIONAL_DEPS.items():
            try:
                # Try to import the package
                module = importlib.import_module(dep_info['package'])
                
                # Get version
                version = self._get_package_version(dep_info['package'])
                
                # Check version requirement
                min_version = pkg_resources.parse_version(dep_info['min_version'])
                current_version = pkg_resources.parse_version(version)
                
                version_ok = current_version >= min_version
                
                self.optional_deps_status[dep_name] = {
                    'available': True,
                    'version': version,
                    'min_version': dep_info['min_version'],
                    'version_ok': version_ok,
                    'purpose': dep_info['purpose'],
                    'required_for': dep_info['required_for']
                }
                
                if self.verbose:
                    status = "✓" if version_ok else "⚠"
                    print(f"{status} Optional dependency '{dep_name}' ({dep_info['package']}) "
                          f"v{version} is available (min: {dep_info['min_version']})", 
                          file=sys.stderr)
                    
            except ImportError:
                self.optional_deps_status[dep_name] = {
                    'available': False,
                    'purpose': dep_info['purpose'],
                    'required_for': dep_info['required_for']
                }
                
                if self.verbose:
                    print(f"✗ Optional dependency '{dep_name}' ({dep_info['package']}) is not installed", 
                          file=sys.stderr)
        
        return self.optional_deps_status
    
    def get_enhanced_capabilities(self) -> List[str]:
        """Get list of enhanced parsing capabilities based on available optional deps"""
        capabilities = []
        
        for dep_name, status in self.optional_deps_status.items():
            if status.get('available') and status.get('version_ok', False):
                capabilities.extend(status.get('required_for', []))
        
        return list(set(capabilities))
    
    def generate_requirements(self) -> str:
        """Generate requirements.txt content based on current dependencies"""
        lines = ["# dscli Python Parser Requirements"]
        lines.append("# Generated by dependency checker\n")
        
        lines.append("# Required dependencies (built-in):")
        for dep_name, module_name in self.REQUIRED_DEPS.items():
            lines.append(f"# {dep_name} ({module_name})")
        
        lines.append("\n# Optional dependencies for enhanced parsing:")
        for dep_name, dep_info in self.OPTIONAL_DEPS.items():
            status = self.optional_deps_status.get(dep_name, {})
            if status.get('available') and status.get('version_ok', False):
                lines.append(f"{dep_info['package']}>={dep_info['min_version']}  # ✓ Installed")
            else:
                lines.append(f"# {dep_info['package']}>={dep_info['min_version']}  # Optional: {dep_info['purpose']}")
        
        return "\n".join(lines)
    
    def check_all(self) -> Dict[str, Any]:
        """Perform complete dependency check"""
        required_ok = self.check_required_deps()
        optional_status = self.check_optional_deps()
        enhanced_capabilities = self.get_enhanced_capabilities()
        
        return {
            'required_deps_ok': required_ok,
            'missing_required_deps': self.missing_deps,
            'optional_deps': optional_status,
            'enhanced_capabilities': enhanced_capabilities,
            'python_version': sys.version,
            'python_executable': sys.executable
        }
    
    def _get_module_version(self, module_name: str) -> str:
        """Get version of a module"""
        try:
            module = importlib.import_module(module_name)
            if hasattr(module, '__version__'):
                return module.__version__
            elif hasattr(module, 'version'):
                return module.version
            else:
                return 'unknown'
        except:
            return 'built-in'
    
    def _get_package_version(self, package_name: str) -> str:
        """Get version of a package using pkg_resources"""
        try:
            return pkg_resources.get_distribution(package_name).version
        except:
            # Fallback to module attribute
            try:
                module = importlib.import_module(package_name)
                if hasattr(module, '__version__'):
                    return module.__version__
                elif hasattr(module, 'version'):
                    return module.version
            except:
                pass
            return 'unknown'


def check_dependencies_command():
    """Command-line interface for dependency checking"""
    import argparse
    
    parser = argparse.ArgumentParser(description='Check dscli Python parser dependencies')
    parser.add_argument('--verbose', '-v', action='store_true', help='Verbose output')
    parser.add_argument('--json', '-j', action='store_true', help='Output as JSON')
    parser.add_argument('--requirements', '-r', action='store_true', 
                       help='Generate requirements.txt content')
    
    args = parser.parse_args()
    
    checker = DependencyChecker(verbose=args.verbose)
    result = checker.check_all()
    
    if args.requirements:
        print(checker.generate_requirements())
    elif args.json:
        print(json.dumps(result, indent=2))
    else:
        # Human-readable output
        print("=" * 60)
        print("dscli Python Parser Dependency Check")
        print("=" * 60)
        
        print(f"\nPython: {result['python_version']}")
        print(f"Executable: {result['python_executable']}")
        
        print(f"\nRequired Dependencies: {'✓ ALL OK' if result['required_deps_ok'] else '✗ MISSING'}")
        
        if result['missing_required_deps']:
            print("\nMissing required dependencies:")
            for dep in result['missing_required_deps']:
                print(f"  ✗ {dep['name']} ({dep['module']}): {dep['error']}")
        
        print("\nOptional Dependencies:")
        for dep_name, status in result['optional_deps'].items():
            if status['available']:
                version_info = f"v{status['version']}" if 'version' in status else "available"
                version_ok = "✓" if status.get('version_ok', False) else "⚠"
                print(f"  {version_ok} {dep_name}: {version_info} - {status['purpose']}")
            else:
                print(f"  ✗ {dep_name}: Not installed - {status['purpose']}")
        
        if result['enhanced_capabilities']:
            print(f"\nEnhanced Capabilities Available:")
            for capability in result['enhanced_capabilities']:
                print(f"  ✓ {capability}")
        
        print("\n" + "=" * 60)


if __name__ == '__main__':
    check_dependencies_command()