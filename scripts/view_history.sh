#!/bin/bash
# 安全的历史对话查看脚本
# 只读操作，自动备份，不修改原始数据库

set -e  # 遇到错误立即退出

DB_PATH="$HOME/.dscli/sqlite.db"
BACKUP_DIR="$HOME/.dscli/backups"
EXPORT_DIR="./history_exports"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查数据库文件是否存在
check_database() {
    if [ ! -f "$DB_PATH" ]; then
        print_error "数据库文件不存在: $DB_PATH"
        exit 1
    fi
    print_info "找到数据库文件: $DB_PATH"
}

# 创建备份
create_backup() {
    mkdir -p "$BACKUP_DIR"
    local timestamp=$(date +"%Y%m%d_%H%M%S")
    local backup_file="$BACKUP_DIR/sqlite_backup_${timestamp}.db"
    
    cp "$DB_PATH" "$backup_file"
    print_success "已创建备份: $backup_file"
    echo "$backup_file"
}

# 显示基本统计
show_stats() {
    print_info "数据库统计信息:"
    echo "----------------------------------------"
    
    sqlite3 "$DB_PATH" << 'EOF'
.headers on
.mode column

-- 基本统计
SELECT 
    COUNT(*) as "总消息数",
    COUNT(DISTINCT session_id) as "对话会话数",
    MIN(created_at) as "最早消息",
    MAX(created_at) as "最新消息"
FROM messages;

-- 角色分布
SELECT 
    role as "角色",
    COUNT(*) as "消息数",
    ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*) FROM messages), 2) as "百分比%"
FROM messages
GROUP BY role
ORDER BY COUNT(*) DESC;

-- 每日统计
SELECT 
    DATE(created_at) as "日期",
    COUNT(*) as "消息数",
    COUNT(DISTINCT session_id) as "会话数"
FROM messages
GROUP BY DATE(created_at)
ORDER BY DATE(created_at) DESC
LIMIT 7;
EOF
}

# 列出所有会话
list_sessions() {
    print_info "所有对话会话:"
    echo "----------------------------------------"
    
    sqlite3 "$DB_PATH" << 'EOF'
.headers on
.mode column

SELECT 
    session_id as "会话ID",
    COUNT(*) as "消息数",
    MIN(created_at) as "开始时间",
    MAX(created_at) as "结束时间"
FROM messages
GROUP BY session_id
ORDER BY session_id DESC;
EOF
}

# 预览指定会话
preview_session() {
    local session_id=$1
    local limit=${2:-10}
    
    print_info "预览会话 $session_id (前$limit条消息):"
    echo "----------------------------------------"
    
    sqlite3 "$DB_PATH" << EOF
.headers on
.mode column

SELECT 
    id as "消息ID",
    role as "角色",
    CASE 
        WHEN LENGTH(content) > 50 THEN SUBSTR(content, 1, 50) || '...'
        ELSE content
    END as "内容预览",
    created_at as "时间"
FROM messages
WHERE session_id = $session_id
ORDER BY created_at
LIMIT $limit;
EOF
}

# 导出会话到文件
export_session() {
    local session_id=$1
    local export_dir="${EXPORT_DIR}/session_${session_id}"
    
    mkdir -p "$export_dir"
    
    print_info "导出会话 $session_id 到: $export_dir"
    
    # 导出为CSV
    sqlite3 -header -csv "$DB_PATH" "SELECT * FROM messages WHERE session_id = $session_id ORDER BY created_at;" > "$export_dir/messages.csv"
    
    # 导出为JSON
    sqlite3 "$DB_PATH" << EOF > "$export_dir/messages.json"
.mode json
SELECT * FROM messages WHERE session_id = $session_id ORDER BY created_at;
EOF
    
    # 导出为可读文本
    sqlite3 "$DB_PATH" << EOF > "$export_dir/conversation.txt"
SELECT 
    '[' || created_at || '] ' || UPPER(role) || ':' || CHAR(10) ||
    CASE 
        WHEN LENGTH(content) > 500 THEN SUBSTR(content, 1, 500) || '...'
        ELSE content
    END || CHAR(10) || '---' || CHAR(10)
FROM messages
WHERE session_id = $session_id
ORDER BY created_at;
EOF
    
    print_success "导出完成:"
    echo "  CSV: $export_dir/messages.csv"
    echo "  JSON: $export_dir/messages.json"
    echo "  文本: $export_dir/conversation.txt"
}

# 显示帮助
show_help() {
    echo "安全历史对话查看器"
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  stats              显示数据库统计信息"
    echo "  list               列出所有对话会话"
    echo "  preview <ID>       预览指定会话"
    echo "  export <ID>        导出指定会话到文件"
    echo "  all                执行所有安全操作"
    echo "  help               显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 stats           查看统计"
    echo "  $0 list            列出所有会话"
    echo "  $0 preview 1       预览会话1"
    echo "  $0 export 1        导出会话1"
    echo "  $0 all             执行完整的安全查看流程"
}

# 完整的安全查看流程
full_safe_process() {
    print_info "开始安全历史查看流程"
    echo "========================================"
    
    # 1. 检查数据库
    check_database
    
    # 2. 创建备份
    create_backup
    
    # 3. 显示统计
    show_stats
    
    # 4. 列出会话
    list_sessions
    
    print_success "安全查看流程完成"
    print_warning "所有操作均为只读，原始数据库未被修改"
}

# 主函数
main() {
    echo "🔒 安全历史对话查看器"
    echo "========================================"
    
    case "$1" in
        "stats")
            check_database
            create_backup
            show_stats
            ;;
        "list")
            check_database
            create_backup
            list_sessions
            ;;
        "preview")
            if [ -z "$2" ]; then
                print_error "请提供会话ID"
                show_help
                exit 1
            fi
            check_database
            create_backup
            preview_session "$2" "$3"
            ;;
        "export")
            if [ -z "$2" ]; then
                print_error "请提供会话ID"
                show_help
                exit 1
            fi
            check_database
            create_backup
            export_session "$2"
            ;;
        "all")
            full_safe_process
            ;;
        "help"|"")
            show_help
            ;;
        *)
            print_error "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
}

# 运行主函数
main "$@"