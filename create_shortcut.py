import os, sys

try:
    import win32com.client
except ImportError:
    import subprocess
    subprocess.check_call([sys.executable, "-m", "pip", "install", "pywin32", "-q"])
    import win32com.client

desktop = os.path.join(os.path.expanduser("~"), "Desktop")
exe_path = r"e:\exe\dist\ClaudeScheduler.exe"
icon_path = r"e:\exe\scheduler.ico"
shortcut_path = os.path.join(desktop, "Claude Scheduler.lnk")

shell = win32com.client.Dispatch("WScript.Shell")
sc = shell.CreateShortCut(shortcut_path)
sc.TargetPath   = exe_path
sc.WorkingDirectory = r"e:\exe\dist"
sc.IconLocation = icon_path
sc.Description  = "Hen gio gui prompt vao Claude Code"
sc.save()

print(f"Shortcut tao thanh cong: {shortcut_path}")
