import json
import urllib.request
import urllib.error
import os
import sys
import threading
import time
import subprocess
from typing import Callable, Optional

GITHUB_REPO = "thydynh03/claude_suite"

class AutoUpdater:
    def __init__(self, current_version: str):
        self.current_version = current_version
        self.api_url = f"https://api.github.com/repos/{GITHUB_REPO}/releases/latest"
        
    def check_for_updates(self) -> dict:
        """
        Kiểm tra phiên bản mới trên GitHub.
        Trả về dict: {"has_update": bool, "version": str, "download_url": str, "release_notes": str}
        """
        try:
            req = urllib.request.Request(self.api_url, headers={'User-Agent': 'Mozilla/5.0'})
            with urllib.request.urlopen(req, timeout=10) as response:
                data = json.loads(response.read().decode())
                
                latest_version = data.get("tag_name", "")
                if latest_version and latest_version != self.current_version:
                    # Find asset named ClaudeSuite.exe
                    download_url = None
                    for asset in data.get("assets", []):
                        if asset.get("name") == "ClaudeSuite.exe":
                            download_url = asset.get("browser_download_url")
                            break
                            
                    if download_url:
                        return {
                            "has_update": True,
                            "version": latest_version,
                            "download_url": download_url,
                            "release_notes": data.get("body", "")
                        }
        except Exception as e:
            print(f"Update check failed: {e}")
            
        return {"has_update": False}

    def download_and_install(self, download_url: str, on_progress: Callable[[int, int], None], on_complete: Callable[[bool, str], None]):
        """
        Tải xuống bản cập nhật và cài đặt trong một thread riêng.
        """
        def _worker():
            try:
                temp_exe = "ClaudeSuite_update.exe"
                if os.path.exists(temp_exe):
                    try:
                        os.remove(temp_exe)
                    except:
                        pass
                        
                req = urllib.request.Request(download_url, headers={'User-Agent': 'Mozilla/5.0'})
                with urllib.request.urlopen(req, timeout=30) as response:
                    total_size = int(response.headers.get('content-length', 0))
                    downloaded = 0
                    chunk_size = 8192
                    
                    with open(temp_exe, 'wb') as out_file:
                        while True:
                            chunk = response.read(chunk_size)
                            if not chunk:
                                break
                            out_file.write(chunk)
                            downloaded += len(chunk)
                            if on_progress:
                                on_progress(downloaded, total_size)
                                
                # Download complete, generate updater.bat and execute
                self._apply_update(temp_exe)
                if on_complete:
                    on_complete(True, "Cập nhật thành công! Đang khởi động lại...")
            except Exception as e:
                if on_complete:
                    on_complete(False, f"Lỗi tải xuống: {e}")

        threading.Thread(target=_worker, daemon=True).start()
        
    def _apply_update(self, temp_exe: str):
        """
        Tạo file updater.bat để lách ghi đè file .exe đang chạy và tự khởi động lại.
        """
        current_exe = sys.executable
        if not getattr(sys, 'frozen', False):
            # Running from source, testing mock
            current_exe = "ClaudeSuite.exe" # mock
            
        bat_content = f"""@echo off
echo Dang cap nhat ClaudeSuite... Vui long doi vai giay...
timeout /t 2 /nobreak > NUL
move /y "{temp_exe}" "{current_exe}"
start "" "{current_exe}"
del "%~f0"
"""
        bat_file = "updater.bat"
        with open(bat_file, "w") as f:
            f.write(bat_content)
            
        subprocess.Popen(bat_file, shell=True)
        # Exit current app
        time.sleep(0.5)
        os._exit(0)
