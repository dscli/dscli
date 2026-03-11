#!/bin/bash

# 笔记管理脚本
# 用于创建、更新、查找笔记

set -e

NOTES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INDEX_FILE="$NOTES_DIR/INDEX.md"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 显示帮助
show_help() {
    cat << EOF
笔记管理脚本

用法: $0 [命令] [参数]

命令:
  create <标题>     创建新笔记
  list [状态]       列出笔记（可选：进行中/已完成/已废弃）
  search <关键词>   搜索笔记
  stats             显示统计信息
  update-index      更新索引文件
  help              显示此帮助信息

示例:
  $0 create "性能优化方案"
  $0 list 进行中
  $0 search shell
  $0 stats
EOF
}

# 创建新笔记
create_note() {
    local title="$1"
    if [[ -z "$title" ]]; then
        log_error "请提供笔记标题"
        show_help
        exit 1
    fi
    
    # 生成文件名（小写，空格替换为连字符）
    local filename=$(echo "$title" | tr '[:upper:]' '[:lower:]' | tr ' ' '-' | tr -cd '[:alnum:]-')
    filename="${filename}.md"
    local filepath="$NOTES_DIR/$filename"
    
    if [[ -f "$filepath" ]]; then
        log_error "笔记已存在: $filename"
        exit 1
    fi
    
    # 获取当前日期
    local current_date=$(date +%Y-%m-%d)
    
    # 创建笔记文件
    cat > "$filepath" << EOF
---
title: "$title"
date: "$current_date"
author: "$USER"
status: "进行中"
tags: ["未分类"]
priority: "中"
---

# $title

## 概述
- 创建时间：$current_date
- 作者：$USER
- 状态：进行中
- 相关代码：
- 相关 Issue：

## 背景
[问题描述、需求说明]

## 内容
[详细内容]

## 结论
[总结、下一步行动]

## 相关链接
- [相关文档]
- [相关代码]
- [相关讨论]

---
*最后更新：$current_date*
*记录者：$USER*
*状态：进行中*
EOF
    
    log_success "创建笔记: $filename"
    log_info "文件路径: $filepath"
    
    # 提示更新索引
    echo
    log_warning "请记得更新索引文件: $INDEX_FILE"
}

# 列出笔记
list_notes() {
    local status_filter="$1"
    
    log_info "笔记列表:"
    echo "----------------------------------------"
    
    for file in "$NOTES_DIR"/*.md; do
        if [[ "$file" == "$INDEX_FILE" ]] || [[ "$file" == "$NOTES_DIR/README.md" ]]; then
            continue
        fi
        
        local filename=$(basename "$file")
        local title=$(grep -m1 '^title:' "$file" | cut -d'"' -f2 2>/dev/null || echo "无标题")
        local date=$(grep -m1 '^date:' "$file" | cut -d'"' -f2 2>/dev/null || echo "未知日期")
        local status=$(grep -m1 '^status:' "$file" | cut -d'"' -f2 2>/dev/null || echo "未知状态")
        local tags=$(grep -m1 '^tags:' "$file" | cut -d'[' -f2 | cut -d']' -f1 2>/dev/null || echo "无标签")
        
        if [[ -n "$status_filter" ]] && [[ "$status" != "$status_filter" ]]; then
            continue
        fi
        
        # 状态颜色
        local status_color=$NC
        case "$status" in
            "进行中") status_color=$YELLOW ;;
            "已完成") status_color=$GREEN ;;
            "已废弃") status_color=$RED ;;
        esac
        
        echo -e "${BLUE}$filename${NC}"
        echo -e "  标题: $title"
        echo -e "  日期: $date"
        echo -e "  状态: ${status_color}$status${NC}"
        echo -e "  标签: $tags"
        echo "----------------------------------------"
    done
}

# 搜索笔记
search_notes() {
    local keyword="$1"
    if [[ -z "$keyword" ]]; then
        log_error "请提供搜索关键词"
        show_help
        exit 1
    fi
    
    log_info "搜索关键词: $keyword"
    echo "----------------------------------------"
    
    local found=0
    for file in "$NOTES_DIR"/*.md; do
        if [[ "$file" == "$INDEX_FILE" ]] || [[ "$file" == "$NOTES_DIR/README.md" ]]; then
            continue
        fi
        
        if grep -qi "$keyword" "$file"; then
            found=1
            local filename=$(basename "$file")
            local title=$(grep -m1 '^title:' "$file" | cut -d'"' -f2 2>/dev/null || echo "无标题")
            
            echo -e "${BLUE}$filename${NC} - $title"
            
            # 显示匹配行
            grep -i "$keyword" "$file" | head -3 | while read line; do
                echo "  ... ${line:0:80}..."
            done
            
            echo "----------------------------------------"
        fi
    done
    
    if [[ $found -eq 0 ]]; then
        log_warning "未找到包含关键词 '$keyword' 的笔记"
    fi
}

# 显示统计信息
show_stats() {
    local total=0
    local in_progress=0
    local completed=0
    local abandoned=0
    
    for file in "$NOTES_DIR"/*.md; do
        if [[ "$file" == "$INDEX_FILE" ]] || [[ "$file" == "$NOTES_DIR/README.md" ]]; then
            continue
        fi
        
        ((total++))
        local status=$(grep -m1 '^status:' "$file" | cut -d'"' -f2 2>/dev/null || echo "未知")
        
        case "$status" in
            "进行中") ((in_progress++)) ;;
            "已完成") ((completed++)) ;;
            "已废弃") ((abandoned++)) ;;
        esac
    done
    
    echo "📊 笔记统计信息"
    echo "----------------------------------------"
    echo -e "总笔记数: ${BLUE}$total${NC}"
    echo -e "进行中: ${YELLOW}$in_progress${NC}"
    echo -e "已完成: ${GREEN}$completed${NC}"
    echo -e "已废弃: ${RED}$abandoned${NC}"
    echo "----------------------------------------"
    
    # 显示标签统计
    echo
    echo "🏷️ 标签统计:"
    echo "----------------------------------------"
    
    declare -A tag_counts
    for file in "$NOTES_DIR"/*.md; do
        if [[ "$file" == "$INDEX_FILE" ]] || [[ "$file" == "$NOTES_DIR/README.md" ]]; then
            continue
        fi
        
        local tags_line=$(grep -m1 '^tags:' "$file" 2>/dev/null || echo "")
        if [[ -n "$tags_line" ]]; then
            # 提取标签列表
            local tags=$(echo "$tags_line" | sed 's/^tags: \[\(.*\)\]/\1/')
            IFS=',' read -ra tag_array <<< "$tags"
            for tag in "${tag_array[@]}"; do
                tag=$(echo "$tag" | sed 's/"//g' | xargs)
                if [[ -n "$tag" ]]; then
                    ((tag_counts["$tag"]++))
                fi
            done
        fi
    done
    
    for tag in "${!tag_counts[@]}"; do
        echo -e "  $tag: ${tag_counts[$tag]}"
    done | sort
    echo "----------------------------------------"
}

# 更新索引文件（简单版本）
update_index() {
    log_warning "此功能尚未完全实现"
    log_info "请手动更新: $INDEX_FILE"
    log_info "参考现有笔记的格式"
}

# 主函数
main() {
    local command="$1"
    
    case "$command" in
        "create")
            create_note "$2"
            ;;
        "list")
            list_notes "$2"
            ;;
        "search")
            search_notes "$2"
            ;;
        "stats")
            show_stats
            ;;
        "update-index")
            update_index
            ;;
        "help"|"")
            show_help
            ;;
        *)
            log_error "未知命令: $command"
            show_help
            exit 1
            ;;
    esac
}

# 运行主函数
main "$@"