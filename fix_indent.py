content = open('claude_suite.py', encoding='utf-8').read()

bad_chunk = """            ts_date = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            with open(log_file, "a", encoding="utf-8") as f:
                f.write(f"[{ts_date}] [{level}] {msg}\\n")
        self.current_theme = "dark"
        self._kanban_after_id = None
        self._last_kanban_state = None

    # ── Menu / UI Helper ──────────────────────────────────────────────────"""

good_chunk = """            ts_date = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            with open(log_file, "a", encoding="utf-8") as f:
                f.write(f"[{ts_date}] [{level}] {msg}\\n")"""

content = content.replace(bad_chunk, good_chunk)

bad_init = """        self.log_count  = 0
        self.current_theme = "dark"

    # ── Menu / UI Helper ──────────────────────────────────────────────────"""
    
good_init = """        self.log_count  = 0
        self.current_theme = "dark"
        self._kanban_after_id = None
        self._last_kanban_state = None

    # ── Menu / UI Helper ──────────────────────────────────────────────────"""

content = content.replace(bad_init, good_init)

open('claude_suite.py', 'w', encoding='utf-8').write(content)
