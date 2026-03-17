#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
Count tokens using HuggingFace tokenizers with a tokenizer.json.
Usage:
  python tools/token_count.py /path/to/tokenizer.json < input.txt
Prints a single integer to stdout.
"""
import sys
import io

def main():
    # 强制 stdin 使用 UTF-8 编码，避免 Windows/PowerShell 编码问题
    sys.stdin = io.TextIOWrapper(sys.stdin.buffer, encoding='utf-8')
    
    if len(sys.argv) < 2:
        print("ERROR: 缺少 tokenizer.json 路径参数", file=sys.stderr)
        sys.exit(1)
    
    tok_path = sys.argv[1]
    
    try:
        from tokenizers import Tokenizer  # pip install tokenizers
    except ImportError as e:
        # tokenizers not installed
        print("ERROR: tokenizers 库未安装，请执行以下命令安装:", file=sys.stderr)
        if sys.platform == "win32":
            print("  pip install tokenizers", file=sys.stderr)
        else:
            print("  pip3 install tokenizers", file=sys.stderr)
        sys.exit(1)
    
    try:
        tok = Tokenizer.from_file(tok_path)
        text = sys.stdin.read()
        enc = tok.encode(text)
        print(len(enc.ids))
    except Exception as e:
        # On any error, print to stderr and exit with error
        print(f"ERROR: tokenizer 编码失败: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()

