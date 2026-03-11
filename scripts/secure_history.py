#!/usr/bin/env python3
"""
安全的历史对话查看工具
遵循专家安全建议的实现
"""

import sqlite3
import sys
import argparse
from pathlib import Path
from contextlib import contextmanager
from typing import List, Dict, Any

class SecureHistoryViewer:
    """安全的历史查看器"""
    
    def __init__(self, db_path: Path):
        self.db_path = db_path
        self.validate_database()
    
    def validate_database(self) -> None:
        """验证数据库文件"""
        if not self.db_path.exists():
            raise FileNotFoundError(f"数据库文件不存在: {self.db_path}")
        if not self.db_path.is_file():
            raise ValueError(f"不是有效的文件: {self.db_path}")
    
    @contextmanager
    def read_only_connection(self):
        """安全的只读数据库连接"""
        # 使用只读URI模式，这是最安全的方式
        conn = sqlite3.connect(f"file:{self.db_path}?mode=ro", uri=True)
        conn.row_factory = sqlite3.Row
        try:
            yield conn
        finally:
            conn.close()
    
    def get_stats(self) -> Dict[str, Any]:
        """获取统计信息"""
        with self.read_only_connection() as conn:
            cursor = conn.cursor()
            
            # 基本统计
            cursor.execute("""
                SELECT 
                    COUNT(*) as total_messages,
                    COUNT(DISTINCT session_id) as total_sessions,
                    MIN(created_at) as earliest,
                    MAX(created_at) as latest
                FROM messages
            """)
            stats = dict(cursor.fetchone())
            
            # 角色分布
            cursor.execute("""
                SELECT 
                    role,
                    COUNT(*) as count
                FROM messages
                GROUP BY role
                ORDER BY count DESC
            """)
            roles = [dict(row) for row in cursor.fetchall()]
            
            # 计算百分比
            for role in roles:
                role['percentage'] = role['count'] * 100.0 / stats['total_messages']
            
            return {
                'stats': stats,
                'roles': roles
            }
    
    def list_sessions(self, limit: int = 10) -> List[Dict[str, Any]]:
        """列出会话"""
        with self.read_only_connection() as conn:
            cursor = conn.cursor()
            cursor.execute("""
                SELECT 
                    session_id,
                    COUNT(*) as message_count,
                    MIN(created_at) as start_time,
                    MAX(created_at) as end_time
                FROM messages
                GROUP BY session_id
                ORDER BY session_id DESC
                LIMIT ?
            """, (limit,))
            return [dict(row) for row in cursor.fetchall()]
    
    def preview_session(self, session_id: int, limit: int = 10) -> List[Dict[str, Any]]:
        """预览会话内容"""
        # 验证会话ID
        if not isinstance(session_id, int) or session_id <= 0:
            raise ValueError("会话ID必须是正整数")
        
        with self.read_only_connection() as conn:
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
            return [dict(row) for row in cursor.fetchall()]

def print_stats(stats: Dict[str, Any]) -> None:
    """打印统计信息"""
    print("📊 数据库统计（安全只读模式）")
    print("=" * 50)
    
    s = stats['stats']
    print(f"总消息数: {s['total_messages']}")
    print(f"对话会话数: {s['total_sessions']}")
    print(f"时间范围: {s['earliest']} 到 {s['latest']}")
    
    print("\n👥 角色分布:")
    for role in stats['roles']:
        print(f"  {role['role']}: {role['count']}条 ({role['percentage']:.2f}%)")

def print_sessions(sessions: List[Dict[str, Any]]) -> None:
    """打印会话列表"""
    print("📅 对话会话列表")
    print("=" * 50)
    
    for session in sessions:
        print(f"会话{session['session_id']}:")
        print(f"  消息数: {session['message_count']}")
        print(f"  时间: {session['start_time']} 到 {session['end_time']}")
        print()

def print_preview(messages: List[Dict[str, Any]], session_id: int) -> None:
    """打印会话预览"""
    print(f"🔍 会话 {session_id} 预览")
    print("=" * 50)
    
    for msg in messages:
        print(f"[{msg['created_at']}] {msg['role']}:")
        print(f"  {msg['preview']}")
        print()

def main():
    """主函数"""
    parser = argparse.ArgumentParser(description='安全历史对话查看器')
    parser.add_argument('--stats', action='store_true', help='显示统计信息')
    parser.add_argument('--list', action='store_true', help='列出会话')
    parser.add_argument('--preview', type=int, help='预览指定会话ID')
    parser.add_argument('--limit', type=int, default=10, help='限制显示数量')
    
    args = parser.parse_args()
    
    # 如果没有参数，显示帮助
    if not any([args.stats, args.list, args.preview]):
        parser.print_help()
        return
    
    try:
        db_path = Path.home() / ".dscli" / "sqlite.db"
        viewer = SecureHistoryViewer(db_path)
        
        if args.stats:
            stats = viewer.get_stats()
            print_stats(stats)
        
        if args.list:
            sessions = viewer.list_sessions(args.limit)
            print_sessions(sessions)
        
        if args.preview:
            messages = viewer.preview_session(args.preview, args.limit)
            print_preview(messages, args.preview)
            
    except FileNotFoundError as e:
        print(f"❌ 错误: {e}")
        sys.exit(1)
    except sqlite3.Error as e:
        print(f"❌ 数据库错误: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"❌ 未知错误: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()