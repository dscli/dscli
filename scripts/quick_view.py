#!/usr/bin/env python3
"""
快速安全查看工具
最简单的数据库查看方式
"""

import sqlite3
import sys
from pathlib import Path

def safe_query(db_path, query):
    """执行安全的只读查询"""
    # 创建临时副本
    import tempfile
    import shutil
    
    temp_db = tempfile.NamedTemporaryFile(suffix='.db', delete=False)
    temp_db.close()
    shutil.copy2(db_path, temp_db.name)
    
    try:
        conn = sqlite3.connect(f"file:{temp_db.name}?mode=ro", uri=True)
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()
        cursor.execute(query)
        results = cursor.fetchall()
        conn.close()
        return [dict(row) for row in results]
    finally:
        import os
        os.unlink(temp_db.name)

def main():
    db_path = Path.home() / ".dscli" / "sqlite.db"
    
    if not db_path.exists():
        print("数据库文件不存在")
        return
    
    print("📊 快速数据库查看")
    print("=" * 50)
    
    # 1. 基本统计
    stats = safe_query(db_path, """
        SELECT 
            COUNT(*) as total_messages,
            COUNT(DISTINCT session_id) as total_sessions,
            MIN(created_at) as earliest,
            MAX(created_at) as latest
        FROM messages
    """)[0]
    
    print(f"📈 统计信息:")
    print(f"  总消息数: {stats['total_messages']}")
    print(f"  对话会话数: {stats['total_sessions']}")
    print(f"  时间范围: {stats['earliest']} 到 {stats['latest']}")
    
    # 2. 会话列表
    sessions = safe_query(db_path, """
        SELECT 
            session_id,
            COUNT(*) as message_count,
            MIN(created_at) as start_time
        FROM messages
        GROUP BY session_id
        ORDER BY session_id DESC
        LIMIT 5
    """)
    
    print(f"\n📅 最近5个会话:")
    for session in sessions:
        print(f"  会话{session['session_id']}: {session['message_count']}条消息, 开始于 {session['start_time']}")
    
    # 3. 如果要查看特定会话
    if len(sys.argv) > 1 and sys.argv[1].isdigit():
        session_id = int(sys.argv[1])
        print(f"\n🔍 查看会话 {session_id}:")
        
        messages = safe_query(db_path, f"""
            SELECT 
                role,
                CASE 
                    WHEN LENGTH(content) > 100 THEN SUBSTR(content, 1, 100) || '...'
                    ELSE content
                END as preview,
                created_at
            FROM messages
            WHERE session_id = {session_id}
            ORDER BY created_at
            LIMIT 10
        """)
        
        for msg in messages:
            print(f"  [{msg['created_at']}] {msg['role']}: {msg['preview']}")
    
    print(f"\n✅ 安全查看完成 (只读操作)")

if __name__ == "__main__":
    main()