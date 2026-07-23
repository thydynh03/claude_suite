@echo off
echo Dang xoa proxy environment variables...

REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "ANTHROPIC_AUTH_TOKEN" /f 2>nul && echo Xoa ANTHROPIC_AUTH_TOKEN OK || echo ANTHROPIC_AUTH_TOKEN khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "ANTHROPIC_BASE_URL" /f 2>nul && echo Xoa ANTHROPIC_BASE_URL OK || echo ANTHROPIC_BASE_URL khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "ANTHROPIC_DEFAULT_OPUS_MODEL" /f 2>nul && echo Xoa ANTHROPIC_DEFAULT_OPUS_MODEL OK || echo khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "ANTHROPIC_DEFAULT_SONNET_MODEL" /f 2>nul && echo Xoa ANTHROPIC_DEFAULT_SONNET_MODEL OK || echo khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "ANTHROPIC_DEFAULT_HAIKU_MODEL" /f 2>nul && echo Xoa ANTHROPIC_DEFAULT_HAIKU_MODEL OK || echo khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "CLAUDE_CODE_SUBAGENT_MODEL" /f 2>nul && echo Xoa CLAUDE_CODE_SUBAGENT_MODEL OK || echo khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "API_TIMEOUT_MS" /f 2>nul && echo Xoa API_TIMEOUT_MS OK || echo khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "alwaysThinkingEnabled" /f 2>nul && echo Xoa alwaysThinkingEnabled OK || echo khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC" /f 2>nul && echo OK || echo khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS" /f 2>nul && echo OK || echo khong ton tai
REG DELETE "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS" /f 2>nul && echo OK || echo khong ton tai

echo.
echo HOAN THANH! Mo lai terminal va thu lai: claude -p "hello"
echo.
pause
