# -*- mode: python ; coding: utf-8 -*-


a = Analysis(
    ['e:\\exe\\agent_manager.py'],
    pathex=[],
    binaries=[],
    datas=[('e:\\exe\\modules', 'modules')],
    hiddenimports=['sqlite3', 'win32gui', 'win32con', 'win32api', 'win32clipboard'],
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[],
    noarchive=False,
    optimize=0,
)
pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.datas,
    [],
    name='ClaudeAgentManager',
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,
    upx_exclude=[],
    runtime_tmpdir=None,
    console=False,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
)
