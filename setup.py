#!/usr/bin/env python3
"""
Setup script for dscli Python Parser

This script handles installation of Python dependencies for the embedded parser.
"""

import sys
import os
import subprocess
import json
from pathlib import Path


class ParserSetup:
    """Setup manager for dscli Python parser dependencies"""
    
    def __init__(self):
        self.project_root = Path(__file__).parent
        self.requirements_file = self.project_root / 'requirements.txt'
        self.deps_check_script = self.project_root / 'deps_check.py'
        
    def check_python_version(self) -> bool:
        """Check if Python version is compatible"""
        required_version = (3, 7)
        current_version = sys.version_info[:2]
        
        if current_version < required_version:
            print(f"Error: Python {required_version[0]}.{required_version[1]}+ required, "
                  f"but found {sys.version_info[0]}.{sys.version_info[1]}")
            return False
        
        print(f"✓ Python {sys.version_info[0]}.{sys.version_info[1]}.{sys.version_info[2]} is compatible")
        return True
    
    def check_dependencies(self) -> bool:
        """Check dependencies using the dependency checker"""
        if not self.deps_check_script.exists():
            print("Error: Dependency checker script not found")
            return False
        
        try:
            result = subprocess.run(
                [sys.executable, str(self.deps_check_script), '--json'],
                capture_output=True,
                text=True,
                cwd=self.project_root
            )
            
            if result.returncode != 0:
                print("Error running dependency checker:")
                print(result.stderr)
                return False
            
            deps_info = json.loads(result.stdout)
            
            if not deps_info.get('required_deps_ok', False):
                print("✗ Missing required dependencies:")
                for dep in deps_info.get('missing_required_deps', []):
                    print(f"  - {dep['name']} ({dep['module']}): {dep['error']}")
                return False
            
            print("✓ All required dependencies are available")
            
            # Show optional dependencies status
            optional_deps = deps_info.get('optional_deps', {})
            if optional_deps:
                print("\nOptional dependencies:")
                for dep_name, status in optional_deps.items():
                    if status.get('available'):
                        version_ok = "✓" if status.get('version_ok', False) else "⚠"
                        print(f"  {version_ok} {dep_name}: v{status.get('version', 'unknown')} - {status.get('purpose', '')}")
                    else:
                        print(f"  ✗ {dep_name}: Not installed - {status.get('purpose', '')}")
            
            enhanced_caps = deps_info.get('enhanced_capabilities', [])
            if enhanced_caps:
                print(f"\nEnhanced capabilities available:")
                for cap in enhanced_caps:
                    print(f"  ✓ {cap}")
            
            return True
            
        except Exception as e:
            print(f"Error checking dependencies: {e}")
            return False
    
    def install_dependencies(self, upgrade: bool = False) -> bool:
        """Install dependencies from requirements.txt"""
        if not self.requirements_file.exists():
            print("Error: requirements.txt not found")
            return False
        
        print(f"Installing dependencies from {self.requirements_file}...")
        
        pip_cmd = [sys.executable, '-m', 'pip', 'install']
        
        if upgrade:
            pip_cmd.append('--upgrade')
        
        pip_cmd.append('-r')
        pip_cmd.append(str(self.requirements_file))
        
        try:
            result = subprocess.run(
                pip_cmd,
                capture_output=True,
                text=True,
                cwd=self.project_root
            )
            
            if result.returncode != 0:
                print("Error installing dependencies:")
                print(result.stderr)
                return False
            
            print("✓ Dependencies installed successfully")
            print(result.stdout)
            return True
            
        except Exception as e:
            print(f"Error running pip install: {e}")
            return False
    
    def install_optional_deps(self, deps: list = None) -> bool:
        """Install specific optional dependencies"""
        optional_deps_map = {
            'astroid': 'astroid>=3.0.0',
            'javalang': 'javalang>=0.13.0',
            'pycparser': 'pycparser>=2.21',
            'all': ['astroid>=3.0.0', 'javalang>=0.13.0', 'pycparser>=2.21']
        }
        
        if deps is None:
            deps = ['all']
        
        packages_to_install = []
        for dep in deps:
            if dep in optional_deps_map:
                package_spec = optional_deps_map[dep]
                if isinstance(package_spec, list):
                    packages_to_install.extend(package_spec)
                else:
                    packages_to_install.append(package_spec)
            else:
                print(f"Warning: Unknown optional dependency '{dep}'")
        
        if not packages_to_install:
            print("No packages to install")
            return True
        
        print(f"Installing optional dependencies: {', '.join(packages_to_install)}...")
        
        pip_cmd = [sys.executable, '-m', 'pip', 'install']
        pip_cmd.extend(packages_to_install)
        
        try:
            result = subprocess.run(
                pip_cmd,
                capture_output=True,
                text=True,
                cwd=self.project_root
            )
            
            if result.returncode != 0:
                print("Error installing optional dependencies:")
                print(result.stderr)
                return False
            
            print("✓ Optional dependencies installed successfully")
            print(result.stdout)
            return True
            
        except Exception as e:
            print(f"Error running pip install: {e}")
            return False
    
    def run_tests(self) -> bool:
        """Run parser tests"""
        test_script = self.project_root / 'test_parser.py'
        
        if not test_script.exists():
            print("Warning: Test script not found")
            return True
        
        print("Running parser tests...")
        
        try:
            result = subprocess.run(
                [sys.executable, str(test_script)],
                capture_output=True,
                text=True,
                cwd=self.project_root
            )
            
            if result.returncode != 0:
                print("Tests failed:")
                print(result.stderr)
                return False
            
            print("✓ All tests passed")
            print(result.stdout)
            return True
            
        except Exception as e:
            print(f"Error running tests: {e}")
            return False
    
    def show_help(self):
        """Show help information"""
        print("""
dscli Python Parser Setup

Usage:
  python setup.py [command] [options]

Commands:
  check           Check dependencies and Python version
  install         Install all dependencies from requirements.txt
  install-opt     Install optional dependencies [astroid, javalang, pycparser, all]
  test            Run parser tests
  help            Show this help message

Options:
  --upgrade       Upgrade existing packages when installing

Examples:
  python setup.py check
  python setup.py install
  python setup.py install-opt astroid pycparser
  python setup.py install --upgrade
  python setup.py test
        """)
    
    def run(self, args):
        """Run setup command"""
        if not args or args[0] == 'help':
            self.show_help()
            return 0
        
        command = args[0]
        
        # Check Python version first for all commands
        if not self.check_python_version():
            return 1
        
        if command == 'check':
            return 0 if self.check_dependencies() else 1
        
        elif command == 'install':
            upgrade = '--upgrade' in args
            return 0 if self.install_dependencies(upgrade) else 1
        
        elif command == 'install-opt':
            deps = args[1:] if len(args) > 1 else ['all']
            return 0 if self.install_optional_deps(deps) else 1
        
        elif command == 'test':
            # First check dependencies
            if not self.check_dependencies():
                print("\nSome dependencies are missing. Install them first:")
                print("  python setup.py install")
                return 1
            return 0 if self.run_tests() else 1
        
        else:
            print(f"Error: Unknown command '{command}'")
            self.show_help()
            return 1


def main():
    """Main entry point"""
    setup = ParserSetup()
    sys.exit(setup.run(sys.argv[1:]))


if __name__ == '__main__':
    main()