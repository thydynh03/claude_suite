# Contributing to Claude Suite

Thank you for your interest in contributing to **Claude Suite**! This document provides guidelines for developers wishing to contribute features, bug fixes, or enhancements.

---

## 🏛️ Code Architecture Principles

1. **Modular UI & Separation of Concerns**:
   - Keep UI components inside `ui/components/`, `ui/tabs/`, or `ui/modals/`.
   - Keep business logic, database queries, and CLI integration inside `modules/`.
   - Keep dataclass models in `modules/models.py`.

2. **Dual Theme Compliance (User Rule)**:
   - Every UI component MUST support both `dark` and `light` appearance modes.
   - Use tuple colors for CustomTkinter widgets: `fg_color=("light_color", "dark_color")`.

3. **Log Integrity & Type Hints**:
   - Always log background actions using `self._global_log()` or `orchestrator.log()`.
   - Use explicit Python type annotations (`str`, `int`, `Optional`, `List`, `Dict`).

---

## 🛠️ Development Setup

1. **Fork and Clone**:
   ```bash
   git clone https://github.com/your-username/claude-suite.git
   cd claude-suite
   ```

2. **Create a Virtual Environment**:
   ```bash
   python -m venv venv
   source venv/bin/activate  # On Windows: venv\Scripts\activate
   pip install -r requirements.txt
   ```

3. **Run Unit Verification**:
   ```bash
   python -c "import main; print('Imports OK')"
   ```

---

## 💡 How to Add a New UI Tab

1. Create a new tab file in `ui/tabs/new_feature_tab.py`.
2. Define a class inheriting from `ctk.CTkFrame`.
3. Register your tab inside `ui/app.py` in `_build_ui()`.
4. Add a menu trigger inside `_show_view_menu()`.

---

## 📜 Pull Request Checklist

- [ ] Code follows Python PEP 8 style guidelines.
- [ ] No hardcoded file paths (use relative or configurable paths).
- [ ] Tested with both Light and Dark themes.
- [ ] Verified build succeeds with `pyinstaller`.
