#!/usr/bin/env python3
"""
dscli parse.py - File structure parsing using tree-sitter.

Replaces regex-based parsing with proper AST parsing via tree-sitter
grammars for supported languages.  Falls back to regex for others.

Input (stdin):  JSON with "content" and "language" fields.
Output (stdout): JSON with structure info (functions, classes, imports, …).
"""

from __future__ import annotations

import json
import re
import sys
import traceback
from typing import Any, Callable

from tree_sitter import Language, Parser

# ═══════════════════════════════════════════════════════════════════════════════
# Language loading
# ═══════════════════════════════════════════════════════════════════════════════

_LANGUAGES: dict[str, Language] = {}


def _load(name: str, module_name: str) -> None:
    try:
        mod = __import__(module_name)
        _LANGUAGES[name] = Language(mod.language())
    except Exception:
        pass


_load("python",     "tree_sitter_python")
_load("c",          "tree_sitter_c")
_load("cpp",        "tree_sitter_cpp")
_load("go",         "tree_sitter_go")
_load("markdown",   "tree_sitter_markdown")
_load("javascript", "tree_sitter_javascript")
_load("rust",       "tree_sitter_rust")
_load("zig",        "tree_sitter_zig")
_load("java",       "tree_sitter_java")
_load("lua",        "tree_sitter_lua")
_load("ruby",       "tree_sitter_ruby")
_load("bash",       "tree_sitter_bash")
_load("yaml",       "tree_sitter_yaml")
_load("objc",       "tree_sitter_objc")
_load("sql",        "tree_sitter_sql")
_load("json",       "tree_sitter_json")
_load("swift",      "tree_sitter_swift")
_load("make",       "tree_sitter_make")

# PHP uses language_php() instead of language()
try:
    import tree_sitter_php as _tsp
    _LANGUAGES["php"] = Language(_tsp.language_php())
except Exception:
    pass

# Try tree_sitter_typescript if installed
try:
    import tree_sitter_typescript as _tst
    _LANGUAGES["typescript"] = Language(_tst.language_typescript())
    _LANGUAGES["tsx"] = Language(_tst.language_tsx())
except Exception:
    pass


# ═══════════════════════════════════════════════════════════════════════════════
# Helpers
# ═══════════════════════════════════════════════════════════════════════════════

def _text(node, source: bytes) -> str:
    """Extract source text for a node."""
    try:
        return source[node.start_byte:node.end_byte].decode("utf-8")
    except (UnicodeDecodeError, IndexError):
        return ""


def _pos(node) -> tuple[int, int, int, int]:
    """Return (start_line, start_col, end_line, end_col) — all 1‑based."""
    return (
        node.start_point[0] + 1,
        node.start_point[1] + 1,
        node.end_point[0] + 1,
        node.end_point[1] + 1,
    )


def _sym(name: str, kind: str, node) -> dict[str, Any]:
    """Build a symbol dict from a tree-sitter node."""
    sl, sc, el, ec = _pos(node)
    return {
        "name": name,
        "type": kind,
        "lineno": sl,
        "col_offset": sc,
        "end_lineno": el,
        "end_col_offset": ec,
    }


def _find_child(node, *types: str):
    """Return the first *named* child whose type is in *types*."""
    for ch in node.children:
        if ch.is_named and ch.type in types:
            return ch
    return None


def _name(node, source: bytes) -> str:
    """Best-effort name extraction: first identifier‑like child."""
    ident = _find_child(
        node,
        "identifier",
        "field_identifier",
        "type_identifier",
        "namespace_identifier",
        "statement_identifier",
        "simple_identifier",
        "word",
        "constant",
    )
    return _text(ident, source) if ident else ""


def _walk(root, source: bytes, handlers: dict[str, Callable], skip: set[str]):
    """Walk *root* (and recursively its named children).

    *handlers* maps node type → callable(node, source) — called once,
    then children are **not** recursed into.

    *skip* lists node types whose children should also be skipped (e.g.
    function bodies, block statements).
    """

    def _go(node):
        tp = node.type
        if tp in handlers:
            handlers[tp](node, source)
            return
        if tp in skip:
            return
        for ch in node.children:
            if ch.is_named:
                _go(ch)

    for ch in root.children:
        if ch.is_named:
            _go(ch)


def _walk_all(root, source: bytes, handlers: dict[str, Callable]):
    """Like _walk but recurses into matched nodes' children too."""

    def _go(node):
        tp = node.type
        if tp in handlers:
            handlers[tp](node, source)
        for ch in node.children:
            if ch.is_named:
                _go(ch)

    for ch in root.children:
        if ch.is_named:
            _go(ch)


# ═══════════════════════════════════════════════════════════════════════════════
# Per‑language tree-sitter parsers
# ═══════════════════════════════════════════════════════════════════════════════

def _ts_python(root, source: bytes, r: dict):
    skip = {
        "block", "parameters", "argument_list", "lambda", "dictionary",
        "set", "list", "tuple", "parenthesized_expression", "pattern_list",
        "string", "interpolation",
    }

    def handle_func(node, _src):
        n = _name(node, _src)
        if n:
            r["functions"].append(_sym(n, "function", node))

    def handle_class(node, _src):
        n = _name(node, _src)
        if n:
            r["classes"].append(_sym(n, "class", node))

    def handle_import(node, _src):
        for ch in node.children:
            if ch.type == "dotted_name":
                r["imports"].append(_text(ch, _src))

    def handle_import_from(node, _src):
        parts = []
        for ch in node.children:
            if ch.type == "dotted_name":
                parts.append(_text(ch, _src))
        if parts:
            r["imports"].append(".".join(parts))

    handlers = {
        "function_definition":      handle_func,
        "class_definition":         handle_class,
        "import_statement":         handle_import,
        "import_from_statement":    handle_import_from,
        "future_import_statement":  handle_import_from,
    }
    _walk(root, source, handlers, skip)


def _ts_c(root, source: bytes, r: dict):
    skip = {
        "compound_statement", "field_declaration_list", "enumerator_list",
        "parameter_list", "argument_list", "initializer_list",
    }

    def handle_func(node, _src):
        decl = _find_child(node, "function_declarator")
        name = _name(decl, _src) if decl else _name(node, _src)
        if name:
            r["functions"].append(_sym(name, "function", node))

    def handle_struct(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "struct", node))

    def handle_union(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "union", node))

    def handle_enum(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "enum", node))

    def handle_typedef(node, _src):
        # type_definition: alias is the LAST type_identifier
        last = ""
        for ch in node.children:
            if ch.type == "type_identifier":
                last = _text(ch, _src)
        if last:
            r["classes"].append(_sym(last, "typedef", node))

    def handle_include(node, _src):
        for ch in node.children:
            if ch.type == "system_lib_string":
                r["imports"].append(_text(ch, _src))
                return
            elif ch.type == "string_content":
                r["imports"].append('"' + _text(ch, _src) + '"')
                return

    def handle_define(node, _src):
        ident = _find_child(node, "identifier")
        if ident:
            r["classes"].append(_sym(_text(ident, _src), "macro", node))

    handlers = {
        "function_definition": handle_func,
        "struct_specifier":    handle_struct,
        "union_specifier":     handle_union,
        "enum_specifier":      handle_enum,
        "type_definition":     handle_typedef,
        "preproc_include":     handle_include,
        "preproc_def":         handle_define,
    }
    _walk(root, source, handlers, skip)


def _ts_cpp(root, source: bytes, r: dict):
    skip = {
        "compound_statement", "field_declaration_list", "enumerator_list",
        "parameter_list", "argument_list", "initializer_list",
    }

    def handle_func(node, _src):
        decl = _find_child(node, "function_declarator")
        name = _name(decl, _src) if decl else _name(node, _src)
        if name:
            r["functions"].append(_sym(name, "function", node))

    def handle_struct(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "struct", node))

    def handle_class(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "class", node))

    def handle_union(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "union", node))

    def handle_enum(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "enum", node))

    def handle_namespace(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "namespace", node))

    def handle_typedef(node, _src):
        last = ""
        for ch in node.children:
            if ch.type == "type_identifier":
                last = _text(ch, _src)
        if last:
            r["classes"].append(_sym(last, "typedef", node))

    def handle_include(node, _src):
        for ch in node.children:
            if ch.type == "system_lib_string":
                r["imports"].append(_text(ch, _src))
                return
            elif ch.type == "string_content":
                r["imports"].append('"' + _text(ch, _src) + '"')
                return

    def handle_define(node, _src):
        ident = _find_child(node, "identifier")
        if ident:
            r["classes"].append(_sym(_text(ident, _src), "macro", node))

    handlers = {
        "function_definition":   handle_func,
        "struct_specifier":      handle_struct,
        "class_specifier":       handle_class,
        "union_specifier":       handle_union,
        "enum_specifier":        handle_enum,
        "namespace_definition":  handle_namespace,
        "type_definition":       handle_typedef,
        "preproc_include":       handle_include,
        "preproc_def":           handle_define,
    }
    _walk(root, source, handlers, skip)


def _ts_go(root, source: bytes, r: dict):
    skip = {
        "block", "parameter_list", "argument_list", "literal_value",
        "interpreted_string_literal", "raw_string_literal",
    }

    def handle_func(node, _src):
        name = _name(node, _src)
        if name:
            r["functions"].append(_sym(name, "function", node))

    def handle_method(node, _src):
        name = _name(node, _src)
        if name:
            recv_type = ""
            recv = _find_child(node, "parameter_list")
            if recv:
                tid = _find_child(recv, "type_identifier")
                if tid:
                    recv_type = _text(tid, _src)
            sym = _sym(name, "method", node)
            if recv_type:
                sym["receiver"] = recv_type
            r["functions"].append(sym)

    def handle_type(node, _src):
        spec = _find_child(node, "type_spec")
        if not spec:
            return
        name = _name(spec, _src)
        if not name:
            return
        if _find_child(spec, "struct_type"):
            kind = "struct"
        elif _find_child(spec, "interface_type"):
            kind = "interface"
        else:
            kind = "type"
        r["classes"].append(_sym(name, kind, spec))

    def handle_import(node, _src):
        for ch in node.children:
            if ch.type == "import_spec":
                lit = _find_child(ch, "interpreted_string_literal")
                if lit:
                    txt = _text(lit, _src).strip('"')
                    if txt:
                        r["imports"].append(txt)

    handlers = {
        "function_declaration": handle_func,
        "method_declaration":   handle_method,
        "type_declaration":     handle_type,
        "import_declaration":   handle_import,
    }
    _walk(root, source, handlers, skip)


def _ts_javascript(root, source: bytes, r: dict):
    skip = {
        "statement_block", "formal_parameters", "class_body",
        "object", "array", "string", "template_string",
        "parenthesized_expression",
    }

    def handle_func(node, _src):
        name = _name(node, _src)
        if not name:
            return
        kind = "function"
        if _find_child(node, "async"):
            kind = "async_function"
        r["functions"].append(_sym(name, kind, node))

    def handle_class(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "class", node))

    def handle_import(node, _src):
        for ch in node.children:
            if ch.type == "string":
                path = _text(ch, _src).strip("'\"")
                if path:
                    r["imports"].append(path)
                    return

    def handle_lexical(node, _src):
        for ch in node.children:
            if ch.type == "variable_declarator":
                name = _name(ch, _src)
                if not name:
                    continue
                arrow = _find_child(ch, "arrow_function")
                if arrow:
                    r["functions"].append(_sym(name, "function", ch))

    handlers = {
        "function_declaration":             handle_func,
        "generator_function_declaration":   handle_func,
        "class_declaration":                handle_class,
        "import_statement":                 handle_import,
        "lexical_declaration":              handle_lexical,
        "variable_declaration":             handle_lexical,
    }
    _walk(root, source, handlers, skip)


def _ts_typescript(root, source: bytes, r: dict):
    # TypeScript grammar extends JavaScript — reuse JS walker and add TS extras.
    _ts_javascript(root, source, r)

    skip = {
        "statement_block", "formal_parameters", "class_body",
        "object_type", "object", "array", "string", "template_string",
        "parenthesized_expression",
    }

    def handle_interface(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "interface", node))

    def handle_type_alias(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "type_alias", node))

    def handle_enum(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "enum", node))

    handlers = {
        "interface_declaration": handle_interface,
        "type_alias_declaration": handle_type_alias,
        "enum_declaration": handle_enum,
    }
    _walk(root, source, handlers, skip)


def _ts_markdown(root, source: bytes, r: dict):
    # tree-sitter-markdown uses flat inline text with anonymous bracket/paren
    # tokens for links — no named inline_link node.  We fall back to regex on
    # the inline text content to extract links.
    _LINK_RE = re.compile(r"\[([^\]]+)\]\(([^)]+)\)")

    skip: set[str] = set()

    def handle_heading(node, _src):
        level = 1
        for ch in node.children:
            if ch.type.startswith("atx_h") and ch.type.endswith("_marker"):
                try:
                    level = int(ch.type[5])
                except ValueError:
                    pass
                break
        inline = _find_child(node, "inline")
        text = _text(inline, _src) if inline else _text(node, _src)
        r.setdefault("headings", []).append({
            "name": text.strip(),
            "type": f"heading_{level}",
            "lineno": node.start_point[0] + 1,
        })

    def handle_code_block(node, _src):
        lang = ""
        info = _find_child(node, "info_string")
        if info:
            lang = _text(info, _src).strip()
        sl, _sc, el, _ec = _pos(node)
        r.setdefault("code_blocks", []).append({
            "name": f"code_block_{sl}",
            "type": "code_block",
            "lineno": sl,
            "end_lineno": el,
            "language": lang,
        })

    def handle_list_item(node, _src):
        para = _find_child(node, "paragraph")
        text = _text(para, _src) if para else _text(node, _src)
        sl = node.start_point[0] + 1
        r.setdefault("lists", []).append({
            "name": text.strip()[:80],
            "type": "list_item",
            "lineno": sl,
        })

    def handle_link_ref(node, _src):
        label = _find_child(node, "link_label")
        dest = _find_child(node, "link_destination")
        if label and dest:
            r.setdefault("links", []).append({
                "name": _text(label, _src).strip("[]"),
                "type": "link",
                "lineno": node.start_point[0] + 1,
                "url": _text(dest, _src).strip(),
            })

    def handle_inline(node, _src):
        # Scan inline text for [text](url) patterns
        text = _text(node, _src)
        for m in _LINK_RE.finditer(text):
            sl = node.start_point[0] + 1
            r.setdefault("links", []).append({
                "name": m.group(1).strip(),
                "type": "link",
                "lineno": sl,
                "url": m.group(2).strip(),
            })

    handlers = {
        "atx_heading":              handle_heading,
        "setext_heading":           handle_heading,
        "fenced_code_block":        handle_code_block,
        "indented_code_block":      handle_code_block,
        "list_item":                handle_list_item,
        "link_reference_definition": handle_link_ref,
        "inline":                   handle_inline,
    }
    _walk(root, source, handlers, skip)


def _ts_rust(root, source: bytes, r: dict):
    skip = {"block", "parameters", "field_declaration_list", "declaration_list",
            "token_tree", "string_literal", "raw_string_literal"}

    def handle_func(node, _src):
        name = _name(node, _src)
        if name:
            r["functions"].append(_sym(name, "function", node))

    def handle_struct(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "struct", node))

    def handle_enum(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "enum", node))

    def handle_trait(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "trait", node))

    def handle_impl(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "impl", node))

    def handle_use(node, _src):
        # Extract the full path from scoped_identifier or identifier
        path = ""
        for ch in node.children:
            if ch.type in ("scoped_identifier", "identifier", "scoped_use_list"):
                path = _text(ch, _src)
                break
        if path:
            r["imports"].append(path)

    def handle_mod(node, _src):
        name = _name(node, _src)
        if name:
            r["imports"].append(name)

    handlers = {
        "function_item":        handle_func,
        "struct_item":          handle_struct,
        "enum_item":            handle_enum,
        "trait_item":           handle_trait,
        "impl_item":            handle_impl,
        "use_declaration":      handle_use,
        "mod_item":             handle_mod,
    }
    _walk(root, source, handlers, skip)


def _ts_zig(root, source: bytes, r: dict):
    skip = {"block", "parameters", "block_body", "struct_body", "payload",
            "string_literal", "multiline_string_literal"}

    def handle_func(node, _src):
        name = _name(node, _src)
        if name:
            r["functions"].append(_sym(name, "function", node))

    def handle_var(node, _src):
        # Variable declarations may be imports (@import) or structs
        ident = _find_child(node, "identifier")
        if not ident:
            return
        name_val = _text(ident, _src)
        # Check if it's a struct definition
        if _find_child(node, "struct_declaration"):
            r["classes"].append(_sym(name_val, "struct", node))
            return
        # Check if it's an @import
        bf = _find_child(node, "builtin_function")
        if bf and "@import" in _text(bf, _src):
            r["imports"].append(name_val)
            return
        # Regular variable
        r["classes"].append(_sym(name_val, "variable", node))

    handlers = {
        "function_declaration":  handle_func,
        "variable_declaration":  handle_var,
        "test_declaration":      handle_func,
    }
    _walk(root, source, handlers, skip)


def _ts_java(root, source: bytes, r: dict):
    # Use _walk_all so we recurse into class_body to find methods.
    # Method bodies (block) are harmless — they contain no class/interface decls.

    def handle_class(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "class", node))

    def handle_interface(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "interface", node))

    def handle_enum(node, _src):
        name = _name(node, _src)
        if name:
            r["classes"].append(_sym(name, "enum", node))

    def handle_method(node, _src):
        name = _name(node, _src)
        if name:
            r["functions"].append(_sym(name, "method", node))

    def handle_constructor(node, _src):
        name = _name(node, _src)
        if name:
            r["functions"].append(_sym(name, "constructor", node))

    def handle_import(node, _src):
        si = _find_child(node, "scoped_identifier")
        if si:
            r["imports"].append(_text(si, _src))
        else:
            ident = _find_child(node, "identifier")
            if ident:
                r["imports"].append(_text(ident, _src))

    def handle_package(node, _src):
        si = _find_child(node, "scoped_identifier")
        if si:
            r["imports"].append(f"package {_text(si, _src)}")

    handlers = {
        "class_declaration":       handle_class,
        "interface_declaration":   handle_interface,
        "enum_declaration":        handle_enum,
        "method_declaration":      handle_method,
        "constructor_declaration": handle_constructor,
        "import_declaration":      handle_import,
        "package_declaration":     handle_package,
    }
    _walk_all(root, source, handlers)

def _ts_lua(root, source: bytes, r: dict):
    skip = {"block", "parameters", "arguments", "string", "table_constructor",
            "binary_expression", "expression_list"}

    def handle_func(node, _src):
        # Check for method: `function MyTable:method()`
        if _find_child(node, "method_index_expression"):
            n = _name(node, _src)
            if n:
                r["functions"].append(_sym(n, "method", node))
        else:
            n = _name(node, _src)
            if n:
                r["functions"].append(_sym(n, "function", node))

    def handle_local_func(node, _src):
        # `local function name()` — function_declaration with "local" modifier
        for ch in node.children:
            if ch.type == "identifier":
                r["functions"].append(_sym(_text(ch, _src), "function", node))
                return

    def handle_var(node, _src):
        # variable_declaration → assignment_statement → variable_list → identifier
        ident = None
        assign = _find_child(node, "assignment_statement")
        if assign:
            vlist = _find_child(assign, "variable_list")
            if vlist:
                ident = _find_child(vlist, "identifier")
        if not ident:
            return
        name_val = _text(ident, _src)
        # Look for require() call — traverse assignment_statement → expression_list
        fc = None
        elist = _find_child(assign, "expression_list")
        if elist:
            fc = _find_child(elist, "function_call")
        if fc:
            call_name = _find_child(fc, "identifier")
            if call_name and _text(call_name, _src) == "require":
                args = _find_child(fc, "arguments")
                if args:
                    arg_str = _text(args, _src).strip("()'\" \n")
                    if arg_str:
                        r["imports"].append(arg_str)
                        return
        # Regular local variable
        r.setdefault("variables", []).append(_sym(name_val, "variable", node))

    handlers = {
        "function_declaration": handle_func,
        "variable_declaration": handle_var,
    }
    _walk(root, source, handlers, skip)


def _ts_ruby(root, source: bytes, r: dict):
    skip = {"body_statement", "method_parameters", "argument_list",
            "string", "integer", "array", "hash", "binary"}

    def handle_class(node, _src):
        n = _name(node, _src)
        if n:
            r["classes"].append(_sym(n, "class", node))

    def handle_module(node, _src):
        n = _name(node, _src)
        if n:
            r["classes"].append(_sym(n, "module", node))

    def handle_method(node, _src):
        n = _name(node, _src)
        if n:
            r["functions"].append(_sym(n, "method", node))

    def handle_singleton(node, _src):
        n = _name(node, _src)
        if n:
            r["functions"].append(_sym(n, "class_method", node))

    def handle_call(node, _src):
        ident = _find_child(node, "identifier")
        if ident and _text(ident, _src) == "require":
            al = _find_child(node, "argument_list")
            if al:
                arg = _text(al, _src).strip("()'\" \n")
                if arg:
                    r["imports"].append(arg)

    handlers = {
        "class":              handle_class,
        "module":             handle_module,
        "method":             handle_method,
        "singleton_method":   handle_singleton,
        "call":               handle_call,
    }
    _walk_all(root, source, handlers)


def _ts_bash(root, source: bytes, r: dict):
    skip = {"compound_statement", "string", "command", "command_name",
            "variable_name"}

    def handle_func(node, _src):
        n = _name(node, _src)
        if n:
            r["functions"].append(_sym(n, "function", node))

    def handle_var(node, _src):
        name_node = _find_child(node, "variable_name")
        if name_node:
            name = _text(name_node, _src)
            r.setdefault("variables", []).append(_sym(name, "variable", node))

    def handle_source(node, _src):
        cmd = _find_child(node, "command_name")
        if cmd:
            cmd_text = _text(cmd, _src)
            if cmd_text in ("source", "."):
                # The file argument is the next word
                for ch in node.children:
                    if ch.type == "word" and ch is not cmd:
                        file_arg = _text(ch, _src)
                        if file_arg:
                            r["imports"].append(file_arg)
                        break

    handlers = {
        "function_definition": handle_func,
        "variable_assignment": handle_var,
        "command":             handle_source,
    }
    _walk(root, source, handlers, skip)


def _ts_yaml(root, source: bytes, r: dict):
    """Extract top-level mapping keys from YAML documents."""

    def collect(block_map, _src):
        for pair in block_map.children:
            if pair.type != "block_mapping_pair":
                continue
            key_node = _find_child(pair, "flow_node")
            if not key_node:
                continue
            # The key is a plain_scalar → string_scalar
            ps = _find_child(key_node, "plain_scalar")
            if ps:
                ss = _find_child(ps, "string_scalar")
                if ss:
                    name = _text(ss, _src).strip()
                    if name:
                        r.setdefault("keys", []).append(_sym(name, "key", pair))

    def handle_doc(node, _src):
        bn = _find_child(node, "block_node")
        if bn:
            bm = _find_child(bn, "block_mapping")
            if bm:
                collect(bm, _src)

    handlers = {"document": handle_doc}
    skip: set[str] = set()
    _walk(root, source, handlers, skip)


def _ts_objc(root, source: bytes, r: dict):
    skip = {"compound_statement", "method_type", "argument_list",
            "string_literal", "expression_statement"}

    def _add_methods(container, _src):
        """Scan container's children for method_declaration/definition."""
        for ch in container.children:
            if not ch.is_named:
                continue
            if ch.type in ("method_declaration", "method_definition"):
                n = _name(ch, _src)
                if n:
                    r["functions"].append(_sym(n, "method", ch))
            elif ch.type in skip:
                continue
            else:
                _add_methods(ch, _src)

    def handle_interface(node, _src):
        n = _name(node, _src)
        if n:
            r["classes"].append(_sym(n, "interface", node))
        _add_methods(node, _src)

    def handle_implementation(node, _src):
        n = _name(node, _src)
        if n:
            r["classes"].append(_sym(n, "implementation", node))
        _add_methods(node, _src)

    def handle_include(node, _src):
        ss = _find_child(node, "system_lib_string")
        if ss:
            r["imports"].append(_text(ss, _src))
        else:
            sc = _find_child(node, "string_content")
            if sc:
                r["imports"].append('"' + _text(sc, _src) + '"')

    handlers = {
        "class_interface":       handle_interface,
        "class_implementation":  handle_implementation,
        "preproc_include":       handle_include,
    }
    _walk(root, source, handlers, skip)


def _ts_sql(root, source: bytes, r: dict):
    skip = {"column_definitions", "function_arguments", "function_body",
            "select", "from", "where", "binary_expression", "literal"}

    def name_from_ref(node, _src):
        ref = _find_child(node, "object_reference")
        if ref:
            ident = _find_child(ref, "identifier")
            if ident:
                return _text(ident, _src).strip("`\"")
        return _name(node, _src)

    def handle_table(node, _src):
        n = name_from_ref(node, _src)
        if n:
            r["classes"].append(_sym(n, "table", node))

    def handle_view(node, _src):
        n = name_from_ref(node, _src)
        if n:
            r["classes"].append(_sym(n, "view", node))

    def handle_function(node, _src):
        n = name_from_ref(node, _src)
        if n:
            r["functions"].append(_sym(n, "function", node))

    handlers = {
        "create_table":    handle_table,
        "create_view":     handle_view,
        "create_function": handle_function,
    }
    _walk(root, source, handlers, skip)


def _ts_json(root, source: bytes, r: dict):
    """Extract top-level keys from JSON objects."""

    def handle_obj(node, _src):
        for pair in node.children:
            if pair.type != "pair":
                continue
            # First named child is the key string
            key_node = _find_child(pair, "string")
            if key_node:
                sc = _find_child(key_node, "string_content")
                if sc:
                    name = _text(sc, _src).strip('"')
                    if name:
                        r.setdefault("keys", []).append(_sym(name, "key", pair))

    def handle_pair(node, _src):
        # Value side: check for nested objects/arrays
        for ch in node.children:
            if ch.type == "object":
                handle_obj(ch, _src)
            elif ch.type == "array":
                r.setdefault("arrays", []).append(_sym(
                    _text(_find_child(node, "string") or node, _src).strip('"')[:40],
                    "array", ch))

    handlers = {"object": handle_obj, "pair": handle_pair}
    skip: set[str] = set()
    _walk(root, source, handlers, skip)


def _ts_swift(root, source: bytes, r: dict):
    skip = {"function_body", "class_body", "protocol_body", "enum_class_body",
            "statements", "string_literal", "line_string_literal",
            "type_annotation"}

    def handle_import(node, _src):
        ident = _find_child(node, "identifier")
        if ident:
            r["imports"].append(_text(ident, _src))

    def classify_type(node, _src) -> str:
        """Check source to differentiate class/struct/enum."""
        txt = _text(node, _src).lstrip()
        if txt.startswith("struct"):
            return "struct"
        elif txt.startswith("enum"):
            return "enum"
        elif txt.startswith("class"):
            return "class"
        return "type"

    def handle_type(node, _src):
        n = _name(node, _src)
        if n:
            kind = classify_type(node, _src)
            r["classes"].append(_sym(n, kind, node))

    def handle_protocol(node, _src):
        n = _name(node, _src)
        if n:
            r["classes"].append(_sym(n, "protocol", node))

    def handle_func(node, _src):
        n = _name(node, _src)
        if n:
            r["functions"].append(_sym(n, "function", node))

    handlers = {
        "import_declaration":      handle_import,
        "class_declaration":       handle_type,
        "protocol_declaration":    handle_protocol,
        "function_declaration":    handle_func,
    }
    _walk_all(root, source, handlers)


def _ts_php(root, source: bytes, r: dict):
    skip = {"block", "formal_parameters", "argument_list", "string",
            "integer", "binary_expression"}

    def php_name(node, _src):
        # PHP uses a 'name' node for class/interface/trait names
        n_node = _find_child(node, "name")
        if n_node:
            return _text(n_node, _src).lstrip("$")
        return _name(node, _src)

    def handle_class(node, _src):
        n = php_name(node, _src)
        if n:
            r["classes"].append(_sym(n, "class", node))

    def handle_interface(node, _src):
        n = php_name(node, _src)
        if n:
            r["classes"].append(_sym(n, "interface", node))

    def handle_trait(node, _src):
        n = php_name(node, _src)
        if n:
            r["classes"].append(_sym(n, "trait", node))

    def handle_method(node, _src):
        n = php_name(node, _src)
        if n:
            r["functions"].append(_sym(n, "method", node))

    def handle_func(node, _src):
        n = php_name(node, _src)
        if n:
            r["functions"].append(_sym(n, "function", node))

    def handle_use(node, _src):
        # PHP AST: namespace_use_declaration → namespace_use_clause → qualified_name
        clause = _find_child(node, "namespace_use_clause")
        target = clause if clause else node
        si = _find_child(target, "scoped_identifier")
        if si:
            r["imports"].append(_text(si, _src))
            return
        qn = _find_child(target, "qualified_name")
        if qn:
            # Collect all name parts recursively (handles namespace_name → name nesting)
            def _collect_names(n):
                parts = []
                for ch in n.children:
                    if ch.type == "name" and ch.is_named:
                        parts.append(_text(ch, _src))
                    elif ch.type == "namespace_name" and ch.is_named:
                        parts.extend(_collect_names(ch))
                return parts
            parts = _collect_names(qn)
            if parts:
                r["imports"].append("\\".join(parts))
            return
        # Fallback: single name node
        n_node = _find_child(target, "name")
        if n_node:
            r["imports"].append(_text(n_node, _src))

    def handle_namespace(node, _src):
        n = php_name(node, _src)
        if n:
            r.setdefault("namespaces", []).append(_sym(n, "namespace", node))

    handlers = {
        "class_declaration":     handle_class,
        "interface_declaration": handle_interface,
        "trait_declaration":     handle_trait,
        "method_declaration":    handle_method,
        "function_definition":   handle_func,
        "namespace_use_declaration": handle_use,
        "namespace_definition":  handle_namespace,
    }
    _walk_all(root, source, handlers)


def _ts_make(root, source: bytes, r: dict):
    skip: set[str] = set()

    def handle_rule(node, _src):
        targets_node = _find_child(node, "targets")
        if targets_node:
            for ch in targets_node.children:
                if ch.type == "word":
                    name = _text(ch, _src)
                    if name:
                        r.setdefault("targets", []).append(_sym(name, "target", node))

    def handle_var(node, _src):
        n = _name(node, _src)
        if n:
            r.setdefault("variables", []).append(_sym(n, "variable", node))

    def handle_include(node, _src):
        list_node = _find_child(node, "list")
        if list_node:
            for ch in list_node.children:
                if ch.type == "word":
                    r["imports"].append(_text(ch, _src))

    handlers = {
        "rule":                handle_rule,
        "variable_assignment": handle_var,
        "include_directive":   handle_include,
    }
    _walk(root, source, handlers, skip)
# ── Registry ────────────────────────────────────────────────────────────────

_TREE_SITTER_PARSERS: dict[str, Callable] = {
    "python":     _ts_python,
    "c":          _ts_c,
    "cpp":        _ts_cpp,
    "go":         _ts_go,
    "javascript": _ts_javascript,
    "typescript": _ts_typescript,
    "tsx":        _ts_typescript,
    "markdown":   _ts_markdown,
    "rust":       _ts_rust,
    "zig":        _ts_zig,
    "java":       _ts_java,
    "lua":        _ts_lua,
    "ruby":       _ts_ruby,
    "bash":       _ts_bash,
    "yaml":       _ts_yaml,
    "objc":       _ts_objc,
    "sql":        _ts_sql,
    "json":       _ts_json,
    "swift":      _ts_swift,
    "php":        _ts_php,
    "make":       _ts_make,
}

# ═══════════════════════════════════════════════════════════════════════════════
# Regex fallback parsers (for languages without tree-sitter grammars)
# ═══════════════════════════════════════════════════════════════════════════════

def _rx_java(content: str, r: dict):
    """Regex-based Java parser (fallback when tree-sitter unavailable)."""
    for m in re.finditer(r"import\s+([\w.]+(?:\.[\w*]+)?)\s*;", content):
        r["imports"].append(m.group(1))
    for m in re.finditer(
        r"(?:public\s+|private\s+|protected\s+|abstract\s+|final\s+)*"
        r"(class|interface|enum)\s+([A-Za-z_$][A-Za-z0-9_$]*)",
        content,
    ):
        r["classes"].append({"name": m.group(2), "type": m.group(1)})
    for m in re.finditer(
        r"(?:public\s+|private\s+|protected\s+|static\s+|final\s+|"
        r"abstract\s+|synchronized\s+)*"
        r"([A-Za-z_$<>\[\]\s]+)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^)]*\)",
        content,
    ):
        r["functions"].append({
            "name": m.group(2).strip(),
            "type": "method",
            "return_type": m.group(1).strip(),
        })


def _rx_org(content: str, r: dict):
    """Regex-based Org-mode parser."""
    lines = content.split("\n")
    in_code = False
    code_start = 0
    for i, line in enumerate(lines):
        m = re.match(r"^(\*+)\s+(.+)$", line)
        if m:
            r.setdefault("headings", []).append({
                "name": m.group(2).strip(),
                "type": f"heading_{len(m.group(1))}",
                "lineno": i + 1,
            })
        if line.strip().startswith("#+BEGIN_SRC"):
            in_code = True
            code_start = i + 1
        elif line.strip().startswith("#+END_SRC"):
            if in_code:
                in_code = False
                r.setdefault("code_blocks", []).append({
                    "name": f"code_block_{code_start}",
                    "type": "code_block",
                    "lineno": code_start,
                    "end_lineno": i + 1,
                })
        list_m = re.match(r"^(\s*)[-+]\s+(.+)$", line)
        if list_m:
            r.setdefault("lists", []).append({
                "name": list_m.group(2).strip(),
                "type": "list_item",
                "lineno": i + 1,
            })


def _rx_elisp(content: str, r: dict):
    """Regex-based Emacs Lisp parser."""
    lines = content.split("\n")
    for i, line in enumerate(lines):
        ln = i + 1
        s = line.strip()
        m = re.match(r"\(defun\s+([^\s\(]+)", s)
        if m:
            r["functions"].append({"name": m.group(1), "type": "function", "lineno": ln})
            continue
        m = re.match(r"\(defmacro\s+([^\s\(]+)", s)
        if m:
            r.setdefault("macros", []).append({"name": m.group(1), "type": "macro", "lineno": ln})
            continue
        m = re.match(r"\(defvar\s+([^\s\(]+)", s)
        if m:
            r.setdefault("variables", []).append({"name": m.group(1), "type": "variable", "lineno": ln})
            continue
        m = re.match(r"\(defcustom\s+([^\s\(]+)", s)
        if m:
            r.setdefault("custom_variables", []).append({"name": m.group(1), "type": "custom_variable", "lineno": ln})
            continue
        m = re.match(r"\(provide\s+'([^\s\)]+)", s)
        if m:
            r.setdefault("provides", []).append({"name": m.group(1), "type": "provide", "lineno": ln})


def _rx_vimscript(content: str, r: dict):
    """Regex-based Vimscript parser."""
    lines = content.split("\n")
    in_func = False
    func_name = ""
    func_start = 0
    for i, line in enumerate(lines):
        ln = i + 1
        s = line.strip()
        if not s or s.startswith('"'):
            continue
        m = re.match(r"^\s*(?:function!?|def)\s+([A-Za-z_][A-Za-z0-9_:]*)\s*\(", s)
        if m and not in_func:
            func_name = m.group(1)
            in_func = True
            func_start = ln
            continue
        if in_func and s == "endfunction":
            r["functions"].append({
                "name": func_name, "type": "function",
                "lineno": func_start, "end_lineno": ln,
            })
            in_func = False
            continue
        m = re.match(r"^\s*command!\s+([A-Za-z_][A-Za-z0-9_]*)", s)
        if m:
            r.setdefault("commands", []).append({"name": m.group(1), "type": "command", "lineno": ln})
            continue
        m = re.match(r"^\s*let\s+([gs]:)?([A-Za-z_][A-Za-z0-9_]*)\s*=", s)
        if m:
            scope = m.group(1) or ""
            var_type = "global_variable" if scope == "g:" else ("script_variable" if scope == "s:" else "variable")
            r.setdefault("variables", []).append({"name": m.group(2), "type": var_type, "lineno": ln})
            continue
        mm = re.match(
            r"^\s*(n?noremap|i?noremap|v?noremap|x?noremap|s?noremap|o?noremap|t?noremap|"
            r"n?map|i?map|v?map|x?map|s?map|o?map|t?map|map)\s+", s)
        if mm:
            rest = s[mm.end():].strip().split(None, 1)
            lhs, rhs = rest[0] if rest else "", rest[1] if len(rest) > 1 else ""
            r.setdefault("mappings", []).append({
                "name": f"{mm.group(1)} {lhs}", "type": "mapping",
                "lineno": ln, "lhs": lhs, "rhs": rhs,
            })
            continue
        m = re.match(r"^\s*augroup\s+([A-Za-z_][A-Za-z0-9_]*)", s)
        if m:
            r.setdefault("augroups", []).append({"name": m.group(1), "type": "augroup", "lineno": ln})
    if in_func:
        r["functions"].append({
            "name": func_name, "type": "function",
            "lineno": func_start, "end_lineno": len(lines),
        })


def _rx_makefile(content: str, r: dict):
    """Regex-based Makefile parser."""
    for i, line in enumerate(content.split("\n"), 1):
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        m = re.match(r"^([^:#=\s]+)\s*:(.*)$", s)
        if m:
            if m.group(1) == ".PHONY":
                for t in m.group(2).split():
                    r.setdefault("phony_targets", []).append({"name": t, "type": "phony_target", "lineno": i})
            else:
                r.setdefault("targets", []).append({
                    "name": m.group(1), "type": "target",
                    "dependencies": m.group(2).strip(), "lineno": i,
                })
            continue
        m = re.match(r"^([A-Za-z_][A-Za-z0-9_]*)\s*[:?+]?=\s*(.+)$", s)
        if m:
            r.setdefault("variables", []).append({
                "name": m.group(1), "type": "variable",
                "value": m.group(2).strip(), "lineno": i,
            })


def _rx_cmake(content: str, r: dict):
    """Regex-based CMake parser."""
    for i, line in enumerate(content.split("\n"), 1):
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        m = re.match(r"^([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)", s)
        if not m:
            continue
        cmd_name, cmd_args = m.group(1), m.group(2).strip()
        if cmd_name in ("set", "option"):
            vm = re.match(r"([A-Za-z_][A-Za-z0-9_]*)\s+(.+)", cmd_args)
            if vm:
                r.setdefault("variables", []).append({
                    "name": vm.group(1), "type": "variable",
                    "value": vm.group(2).strip(), "lineno": i,
                })
                continue
        r.setdefault("commands", []).append({
            "name": cmd_name, "type": "command", "args": cmd_args, "lineno": i,
        })


def _rx_shell(content: str, r: dict):
    """Regex-based Shell script parser."""
    for i, line in enumerate(content.split("\n"), 1):
        m = re.match(r"^\s*(?:function\s+)?([A-Za-z_][A-Za-z0-9_-]*)\s*\(\s*\)", line)
        if m:
            r["functions"].append({"name": m.group(1), "type": "function", "lineno": i})
            continue
        m = re.match(r"^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)", line)
        if m:
            r.setdefault("variables", []).append({
                "name": m.group(1), "type": "variable",
                "value": m.group(2).strip(), "lineno": i,
            })


# ── Fallback registry ──────────────────────────────────────────────────────

_REGEX_PARSERS: dict[str, Callable] = {
    "java":      _rx_java,      # fallback when tree-sitter java unavailable
    "org":       _rx_org,
    "elisp":     _rx_elisp,
    "vimscript": _rx_vimscript,
    "vim":       _rx_vimscript,
    "cmake":     _rx_cmake,
    "shell":     _rx_shell,
}

# ═══════════════════════════════════════════════════════════════════════════════
# Main
# ═══════════════════════════════════════════════════════════════════════════════

def parse_with_treesitter(lang_name: str, content: str) -> dict | None:
    """Parse *content* with tree-sitter grammar for *lang_name*.

    Returns a result dict on success, or None if the grammar is not
    available or parsing fails.
    """
    if lang_name not in _LANGUAGES or lang_name not in _TREE_SITTER_PARSERS:
        return None

    try:
        lang = _LANGUAGES[lang_name]
        parser = Parser(lang)
        source = content.encode("utf-8")
        tree = parser.parse(source)
    except Exception as exc:
        return {
            "success": False,
            "error": f"Tree-sitter parse error: {exc}",
            "functions": [], "classes": [], "imports": [], "errors": [
                f"Tree-sitter parse error: {exc}",
            ],
        }

    result: dict[str, Any] = {
        "success": True,
        "functions": [],
        "classes": [],
        "imports": [],
        "errors": [],
    }

    try:
        _TREE_SITTER_PARSERS[lang_name](tree.root_node, source, result)
    except Exception as exc:
        result["success"] = False
        result["error"] = f"Tree-sitter walk error: {exc}"
        result["errors"].append(
            f"Tree-sitter walk error: {exc}\n{traceback.format_exc()}"
        )

    return result


def parse_with_regex(lang_name: str, content: str) -> dict | None:
    """Parse *content* with a regex-based parser for *lang_name*.

    Returns a result dict on success, or None if no regex parser exists.
    """
    if lang_name not in _REGEX_PARSERS:
        return None

    result: dict[str, Any] = {
        "success": True,
        "functions": [],
        "classes": [],
        "imports": [],
        "errors": [],
    }

    try:
        _REGEX_PARSERS[lang_name](content, result)
    except Exception as exc:
        result["success"] = False
        result["error"] = f"Regex parse error: {exc}"
        result["errors"].append(f"Regex parse error: {exc}")

    return result


def parse(lang_name: str, content: str) -> dict:
    """Parse *content* as *lang_name*, preferring tree-sitter, then regex."""
    # 1. Try tree-sitter
    ts_result = parse_with_treesitter(lang_name, content)
    if ts_result is not None:
        return ts_result

    # 2. Try regex fallback
    rx_result = parse_with_regex(lang_name, content)
    if rx_result is not None:
        return rx_result

    # 3. Unsupported language
    return {
        "success": False,
        "error": f"Unsupported language: {lang_name}",
        "supported": sorted(set(_TREE_SITTER_PARSERS) | set(_REGEX_PARSERS)),
    }


def main() -> None:
    """Entry point: read JSON from stdin, parse, write JSON to stdout."""
    try:
        raw = sys.stdin.read().strip()
        if not raw:
            print(json.dumps({"error": "No input"}, indent=2))
            sys.exit(1)

        try:
            data = json.loads(raw)
        except json.JSONDecodeError as exc:
            print(json.dumps({"error": f"Invalid JSON: {exc}"}, indent=2))
            sys.exit(1)

        if "content" not in data or "language" not in data:
            print(json.dumps({
                "error": "Missing required fields: content, language",
            }, indent=2))
            sys.exit(1)

        result = parse(data["language"], data["content"])

        # Add dependency info for backward compatibility
        result.setdefault("dependency_info", {
            "dependencies_ok": True,
            "python_version": sys.version,
            "tree_sitter_languages": sorted(_LANGUAGES.keys()),
        })

        print(json.dumps(result, indent=2))

    except Exception as exc:
        print(json.dumps({
            "success": False,
            "error": f"Unexpected error: {exc}",
            "traceback": traceback.format_exc(),
        }, indent=2))
        sys.exit(1)


if __name__ == "__main__":
    main()