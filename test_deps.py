#!/usr/bin/env python3
"""
Test script for dependency checking functionality
"""

import sys
import json
import subprocess
from pathlib import Path


def test_dependency_checker():
    """Test the dependency checker script"""
    print("Testing dependency checker...")
    
    deps_check_script = Path(__file__).parent / 'deps_check.py'
    
    # Test JSON output
    result = subprocess.run(
        [sys.executable, str(deps_check_script), '--json'],
        capture_output=True,
        text=True
    )
    
    if result.returncode != 0:
        print(f"✗ Dependency checker failed: {result.stderr}")
        return False
    
    try:
        deps_info = json.loads(result.stdout)
        print("✓ Dependency checker JSON output is valid")
        
        # Check required fields
        required_fields = ['required_deps_ok', 'optional_deps', 'python_version']
        for field in required_fields:
            if field not in deps_info:
                print(f"✗ Missing required field: {field}")
                return False
        
        print(f"✓ All required fields present")
        
        # Check if required dependencies are OK
        if not deps_info['required_deps_ok']:
            print("✗ Required dependencies are not OK")
            print(f"Missing: {deps_info.get('missing_required_deps', [])}")
            return False
        
        print("✓ Required dependencies are OK")
        
        # Show dependency info
        print(f"\nPython: {deps_info['python_version']}")
        print(f"Enhanced capabilities: {deps_info.get('enhanced_capabilities', [])}")
        
        return True
        
    except json.JSONDecodeError as e:
        print(f"✗ Invalid JSON output: {e}")
        print(f"Output: {result.stdout[:200]}...")
        return False


def test_parser_deps():
    """Test parser dependency checking via stdin"""
    print("\nTesting parser dependency check via stdin...")
    
    parser_script = Path(__file__).parent / 'parse.py'
    
    # Test dependency check action
    test_input = json.dumps({
        'action': 'check_deps'
    })
    
    result = subprocess.run(
        [sys.executable, str(parser_script)],
        input=test_input,
        capture_output=True,
        text=True
    )
    
    if result.returncode != 0:
        print(f"✗ Parser dependency check failed: {result.stderr}")
        return False
    
    try:
        deps_info = json.loads(result.stdout)
        print("✓ Parser dependency check successful")
        
        # Check required fields
        if 'dependencies_ok' not in deps_info:
            print("✗ Missing 'dependencies_ok' field")
            return False
        
        if not deps_info['dependencies_ok']:
            print("✗ Parser reports dependencies not OK")
            return False
        
        print("✓ Parser dependencies are OK")
        print(f"Supported languages: {deps_info.get('supported_languages', [])}")
        
        return True
        
    except json.JSONDecodeError as e:
        print(f"✗ Invalid JSON output from parser: {e}")
        print(f"Output: {result.stdout[:200]}...")
        return False


def test_requirements_file():
    """Test requirements.txt file"""
    print("\nTesting requirements.txt...")
    
    requirements_file = Path(__file__).parent / 'requirements.txt'
    
    if not requirements_file.exists():
        print("✗ requirements.txt not found")
        return False
    
    content = requirements_file.read_text()
    
    # Check for required sections
    if '# dscli Python Parser Requirements' not in content:
        print("✗ Missing header in requirements.txt")
        return False
    
    if '# Core dependencies' not in content:
        print("✗ Missing core dependencies section")
        return False
    
    print("✓ requirements.txt is valid")
    print(f"File size: {len(content)} bytes")
    
    return True


def test_setup_script():
    """Test setup.py script"""
    print("\nTesting setup.py...")
    
    setup_script = Path(__file__).parent / 'setup.py'
    
    if not setup_script.exists():
        print("✗ setup.py not found")
        return False
    
    # Test help command
    result = subprocess.run(
        [sys.executable, str(setup_script), 'help'],
        capture_output=True,
        text=True
    )
    
    if result.returncode != 0:
        print(f"✗ setup.py help failed: {result.stderr}")
        return False
    
    if 'dscli Python Parser Setup' not in result.stdout:
        print("✗ setup.py help output incorrect")
        return False
    
    print("✓ setup.py help command works")
    
    # Test check command
    result = subprocess.run(
        [sys.executable, str(setup_script), 'check'],
        capture_output=True,
        text=True
    )
    
    if result.returncode != 0:
        print(f"✗ setup.py check failed: {result.stderr}")
        return False
    
    print("✓ setup.py check command works")
    
    return True


def main():
    """Run all tests"""
    print("=" * 60)
    print("dscli Python Parser Dependency System Tests")
    print("=" * 60)
    
    tests = [
        ('Dependency Checker', test_dependency_checker),
        ('Parser Dependency Check', test_parser_deps),
        ('Requirements File', test_requirements_file),
        ('Setup Script', test_setup_script),
    ]
    
    results = []
    
    for test_name, test_func in tests:
        print(f"\n{test_name}:")
        try:
            success = test_func()
            results.append((test_name, success))
            print(f"{'✓ PASS' if success else '✗ FAIL'}")
        except Exception as e:
            print(f"✗ ERROR: {e}")
            results.append((test_name, False))
    
    print("\n" + "=" * 60)
    print("Test Summary:")
    print("=" * 60)
    
    passed = 0
    for test_name, success in results:
        status = "✓ PASS" if success else "✗ FAIL"
        print(f"{status} {test_name}")
        if success:
            passed += 1
    
    print(f"\nTotal: {passed}/{len(results)} tests passed")
    
    if passed == len(results):
        print("\n✅ All tests passed!")
        return 0
    else:
        print("\n❌ Some tests failed")
        return 1


if __name__ == '__main__':
    sys.exit(main())