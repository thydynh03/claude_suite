@echo off
chcp 65001 > nul
echo Dang chay Claude Prompt Scheduler...
echo.
python "%~dp0claude_scheduler.py"
if errorlevel 1 (
    echo.
    echo LOI: App gap loi. Xem thong bao phia tren.
    pause
)
