#!/usr/bin/env python3
"""
安全的历史对话查看器
只读操作，不修改原始数据库
"""

import sqlite3
import json
import sys
import os
from datetime import datetime
from pathlib import Path

def create_backup(db_path):
    """创建数据库备份"""
    backup_dir = Path.home() / ".dscli" / "backups"
    backup_dir.mkdir(exist_ok=True)
    
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    backup_path = backup_dir / f"sqlite_backup_{timestamp}.db"
    
    # 复制数据库文件
    import shutil
    shutil.copy2(db_path, backup_path)
    print(f"✅ 已创建备份: {backup_path}")
    return backup_path

def connect_readonly(db_path):
    """以只读模式连接数据库"""
    # 创建副本用于只读查询
    import tempfile
    import shutil
    
    # 创建临时副本
    temp_db = tempfile.NamedTemporaryFile(suffix='.db', delete=False)
    temp_db.close()
    shutil.copy2(db_path, temp_db.name)
    
    # 以只读模式连接临时副本
    conn = sqlite3.connect(f"file:{temp_db.name}?mode=ro", uri=True)
    conn.row_factory = sqlite3.Row
    return conn, temp_db.name

def get_conversation_stats(conn):
    """获取对话统计信息"""
    cursor = conn.cursor()
    
    # 基本统计
    cursor.execute("""
        SELECT 
            COUNT(*) as total_messages,
            COUNT(DISTINCT session_id) as total_sessions,
            MIN(created_at) as earliest_message,
            MAX(created_at) as latest_message
        FROM messages
    """)
    stats = cursor.fetchone()
    
    # 角色分布
    cursor.execute("""
        SELECT 
            role,
            COUNT(*) as count,
            ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*) FROM messages), 2) as percentage
        FROM messages
        GROUP BY role
        ORDER BY count DESC
    """)
    roles = cursor.fetchall()
    
    # 会话列表
    cursor.execute("""
        SELECT 
            session_id,
            COUNT(*) as message_count,
            MIN(created_at) as start_time,
            MAX(created_at) as end_time
        FROM messages
        GROUP BY session_id
        ORDER BY session_id DESC
        LIMIT 10
    """)
    sessions = cursor.fetchall()
    
    return {
        "stats": dict(stats),
        "roles": [dict(r) for r in roles],
        "recent_sessions": [dict(s) for s in sessions]
    }

def preview_conversation(conn, session_id, limit=20):
    """预览指定会话的对话内容"""
    cursor = conn.cursor()
    
    cursor.execute("""
        SELECT 
            id,
            role,
            CASE 
                WHEN LENGTH(content) > 100 THEN SUBSTR(content, 1, 100) || '...'
                ELSE content
            END as preview,
            LENGTH(content) as content_length,
            created_at
        FROM messages
        WHERE session_id = ?
        ORDER BY created_at
        LIMIT ?
    """, (session_id, limit))
    
    messages = cursor.fetchall()
    return [dict(m) for m in messages]

def export_conversation(conn, session_id, output_dir):
    """导出完整对话到文件"""
    cursor = conn.cursor()
    
    # 获取完整对话
    cursor.execute("""
        SELECT 
            id,
            role,
            content,
            tool_call_id,
            tool_calls,
            created_at,
            reasoning_content
        FROM messages
        WHERE session_id = ?
        ORDER BY created_at
    """, (session_id,))
    
    messages = cursor.fetchall()
    
    # 创建输出目录
    output_path = Path(output_dir) / f"conversation_{session_id}"
    output_path.mkdir(parents=True, exist_ok=True)
    
    # 导出为JSON
    json_path = output_path / "conversation.json"
    with open(json_path, 'w', encoding='utf-8') as f:
        json.dump([dict(m) for m in messages], f, ensure_ascii=False, indent=2)
    
    # 导出为可读文本
    txt_path = output_path / "conversation.txt"
    with open(txt_path, 'w', encoding='utf-8') as f:
        f.write(f"对话会话 ID: {session_id}\n")
        f.write("=" * 50 + "\n\n")
        
        for msg in messages:
            msg_dict = dict(msg)
            f.write(f"[{msg_dict['created_at']}] {msg_dict['role'].upper()}:\n")
            f.write(f"{msg_dict['content'][:500]}\n")
            if msg_dict.get('reasoning_content'):
                f.write(f"推理内容: {msg_dict['reasoning_content'][:200]}...\n")
            f.write("-" * 40 + "\n\n")
    
    print(f"✅ 对话已导出到: {output_path}")
    return str(output_path)

def main():
    """主函数"""
    db_path = Path.home() / ".dscli" / "sqlite.db"
    
    if not db_path.exists():
        print(f"❌ 数据库文件不存在: {db_path}")
        return
    
    print("🔒 安全历史对话查看器")
    print("=" * 50)
    
    # 1. 创建备份
    backup_path = create_backup(db_path)
    
    # 2. 以只读模式连接
    conn, temp_db = None, None
    try:
        conn, temp_db = connect_readonly(db_path)
        print("✅ 已建立只读数据库连接")
        
        # 3. 显示统计信息
        print("\n📊 数据库统计:")
        stats = get_conversation_stats(conn)
        
        print(f"总消息数: {stats['stats']['total_messages']}")
        print(f"对话会话数: {stats['stats']['total_sessions']}")
        print(f"时间范围: {stats['stats']['earliest_message']} 到 {stats['stats']['latest_message']}")
        
        print("\n👥 角色分布:")
        for role in stats['roles']:
            print(f"  {role['role']}: {role['count']}条 ({role['percentage']}%)")
        
        print("\n📅 最近会话:")
        for session in stats['recent_sessions']:
            print(f"  会话{session['session_id']}: {session['message_count']}条消息, {session['start_time']}")
        
        # 4. 交互式查看
        print("\n🔍 交互模式:")
        while True:
            print("\n选项:")
            print("  1. 预览会话内容")
            print("  2. 导出完整对话")
            print("  3. 显示更多统计")
            print("  0. 退出")
            
            choice = input("\n请选择 (0-3): ").strip()
            
            if choice == '0':
                break
            elif choice == '1':
                session_id = input("请输入会话ID: ").strip()
                try:
                    session_id = int(session_id)
                    messages = preview_conversation(conn, session_id)
                    print(f"\n会话 {session_id} 预览 (前20条):")
                    for msg in messages:
                        print(f"[{msg['created_at']}] {msg['role']}: {msg['preview']}")
                except ValueError:
                    print("❌ 请输入有效的数字ID")
            elif choice == '2':
                session_id = input("请输入要导出的会话ID: ").strip()
                output_dir = input("导出目录 (默认: ./exports): ").strip() or "./exports"
                try:
                    session_id = int(session_id)
                    export_path = export_conversation(conn, session_id, output_dir)
                    print(f"✅ 导出完成: {export_path}")
                except ValueError:
                    print("❌ 请输入有效的数字ID")
            elif choice == '3':
                # 更多统计信息
                cursor = conn.cursor()
                cursor.execute("""
                    SELECT 
                        DATE(created_at) as date,
                        COUNT(*) as message_count,
                        COUNT(DISTINCT session_id) as session_count
                    FROM messages
                    GROUP BY DATE(created_at)
                    ORDER BY date DESC
                """)
                daily_stats = cursor.fetchall()
                print("\n📅 每日统计:")
                for stat in daily_stats:
                    print(f"  {stat['date']}: {stat['message_count']}条消息, {stat['session_count']}个会话")
            else:
                print("❌ 无效选择")
    
    finally:
        # 清理临时文件
        if conn:
            conn.close()
        if temp_db and os.path.exists(temp_db):
            os.unlink(temp_db)
            print("\n🧹 已清理临时文件")

if __name__ == "__main__":
    main()