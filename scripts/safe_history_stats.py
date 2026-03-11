#!/usr/bin/env python3
"""
真正安全的历史统计工具
只提供最基本的只读统计功能
"""

import sqlite3
import sys
from pathlib import Path

def main():
    """主函数 - 只读统计，没有用户输入，没有文件操作"""
    db_path = Path.home() / ".dscli" / "sqlite.db"
    
    if not db_path.exists():
        print("数据库文件不存在")
        return 1
    
    try:
        # 使用只读模式直接连接（最安全的方式）
        conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
        cursor = conn.cursor()
        
        # 执行安全的只读查询
        cursor.execute("""
            SELECT 
                COUNT(*) as total_messages,
                COUNT(DISTINCT session_id) as total_sessions,
                MIN(created_at) as earliest,
                MAX(created_at) as latest
            FROM messages
        """)
        
        stats = cursor.fetchone()
        
        print("📊 数据库统计（只读模式）")
        print("=" * 40)
        print(f"总消息数: {stats[0]}")
        print(f"对话会话数: {stats[1]}")
        print(f"时间范围: {stats[2]} 到 {stats[3]}")
        
        # 角色分布
        cursor.execute("""
            SELECT 
                role,
                COUNT(*) as count
            FROM messages
            GROUP BY role
            ORDER BY count DESC
        """)
        
        print("\n👥 角色分布:")
        for role, count in cursor.fetchall():
            percentage = count * 100.0 / stats[0]
            print(f"  {role}: {count}条 ({percentage:.2f}%)")
        
        conn.close()
        return 0
        
    except sqlite3.Error as e:
        print(f"❌ 数据库错误: {e}")
        return 1
    except Exception as e:
        print(f"❌ 未知错误: {e}")
        return 1

if __name__ == "__main__":
    sys.exit(main())
