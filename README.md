# Claude Suite — Autonomous AI Engineering Workspace 🚀

Claude Suite is an advanced **Autonomous AI Software Development Control Center** designed to drastically reduce cognitive load for developers. Instead of forcing users to manually prompt AI web interfaces and copy-paste code snippets, Claude Suite acts as your personal AI engineering team.

## ✨ Core Features (V2 Architecture)

- **AI Cockpit:** A simplified, linear 3-step workflow (Context ➔ Prompt ➔ Execution).
- **Interactive File Tree:** Select exactly which files the AI should read as context using intuitive checkboxes, preventing the AI from reading irrelevant files and wasting tokens.
- **Git Auto-Snapshot (Safety Net):** Automatically commits your project to Git before the AI makes any code changes. If the AI hallucinates or breaks your code, you can rollback instantly.
- **Human-in-the-Loop Checkpoints:** For complex architectural tasks, the AI `Architect/BA` will pause execution and prompt a modal for your approval before handing the plan over to the `Coder` agent.
- **Prompt Suggestions (Pill Buttons):** Built-in templates for common tasks like 🐛 *Bug Fixing*, ✨ *Adding Features*, ♻️ *Optimization*, and 📝 *Documentation*.
- **Quick CLI Runner:** Directly execute terminal commands and simple AI prompts with an embedded UI.
- **Webhook Integration:** Built-in HTTP server to receive payloads from external services (e.g., GitHub Webhooks) to autonomously trigger tasks.

## 🛠️ Installation & Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/thydynh03/claude_suite.git
   cd claude_suite
   ```

2. **Install dependencies:**
   ```bash
   pip install -r requirements.txt
   ```

3. **Run the application:**
   ```bash
   python main.py
   ```
   *(Alternatively, run `claude_suite.py` directly)*

## 📦 Automated Builds (CI/CD)

This repository is equipped with GitHub Actions. 
- Every push to the `master` branch automatically triggers a Windows build check.
- Creating a tag starting with `v` (e.g., `v1.0.0`) will automatically build and publish a `.exe` artifact to the GitHub Releases page.

## 📜 License
Internal Project / Proprietary (Update as needed)
