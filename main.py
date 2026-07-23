#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
main.py — Main Entry Point for Claude Suite Enterprise Control Center.
"""

import sys
import io
import os

# Enforce UTF-8 encoding streams for Windows terminal compatibility
if sys.stdout is not None and hasattr(sys.stdout, "buffer") and not sys.stdout.closed:
    try:
        sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')
    except Exception:
        pass

if sys.stderr is not None and hasattr(sys.stderr, "buffer") and not sys.stderr.closed:
    try:
        sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8', errors='replace')
    except Exception:
        pass

sys.path.insert(0, os.path.dirname(__file__))

from claude_suite import ClaudeSuiteApp


def main():
    """Launch Claude Suite Desktop Application."""
    app = ClaudeSuiteApp()
    app.mainloop()


if __name__ == "__main__":
    main()
