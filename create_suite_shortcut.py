import os
import winshell
from win32com.client import Dispatch

desktop = winshell.desktop()
path = os.path.join(desktop, "Claude Suite.lnk")
target = r"e:\exe\dist\ClaudeSuite.exe"
wDir = r"e:\exe"
icon = r"e:\exe\scheduler.ico"

shell = Dispatch('WScript.Shell')
shortcut = shell.CreateShortCut(path)
shortcut.Targetpath = target
shortcut.WorkingDirectory = wDir
if os.path.exists(icon):
    shortcut.IconLocation = icon
shortcut.save()
print(f"Created shortcut at {path}")
