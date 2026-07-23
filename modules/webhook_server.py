"""
webhook_server.py — Built-in HTTP Webhook Server & Notifications Callback
Cho phép ứng dụng nhận Webhook POST (ví dụ từ GitHub Push/PR) để tự động kích hoạt Agent
và gửi thông báo (Notification Callback).
"""

import http.server
import socketserver
import threading
import json
from typing import Callable, Optional

DEFAULT_PORT = 9090


class WebhookRequestHandler(http.server.BaseHTTPRequestHandler):
    on_webhook_payload: Optional[Callable[[dict], None]] = None

    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        post_data = self.rfile.read(content_length)
        
        try:
            payload = json.loads(post_data.decode('utf-8'))
        except Exception:
            payload = {"raw": post_data.decode('utf-8', errors='ignore')}

        if WebhookRequestHandler.on_webhook_payload:
            WebhookRequestHandler.on_webhook_payload(payload)

        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        response = json.dumps({"status": "ok", "message": "Webhook received successfully"})
        self.wfile.write(response.encode('utf-8'))

    def log_message(self, format, *args):
        # Suppress standard HTTP server console logging
        pass


class WebhookServer:
    """
    Background HTTP Webhook listener server.
    """

    def __init__(self, port: int = DEFAULT_PORT, on_payload: Optional[Callable[[dict], None]] = None):
        self.port       = port
        self.on_payload = on_payload
        self._server    = None
        self._thread    = None
        self._running   = False

    def start(self):
        if self._running:
            return
        self._running = True
        WebhookRequestHandler.on_webhook_payload = self.on_payload

        def _run():
            try:
                with socketserver.TCPServer(("", self.port), WebhookRequestHandler) as httpd:
                    self._server = httpd
                    httpd.serve_forever()
            except Exception as e:
                print(f"Webhook server error: {e}")

        self._thread = threading.Thread(target=_run, daemon=True)
        self._thread.start()

    def stop(self):
        if self._server:
            self._server.shutdown()
            self._server = None
        self._running = False
