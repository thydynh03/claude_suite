import re

class QAInterpreter:
    """
    Module xử lý luồng (stream) của AI để dịch các thao tác code khô khan
    thành dạng Timeline tiếng Việt cho người dùng không rành kỹ thuật (Human View),
    đồng thời tạo Báo Cáo Nghiệm Thu cuối cùng.
    """
    
    def __init__(self):
        self.timeline_events = []
        
    def interpret_log(self, log_text: str) -> str:
        """
        Nhận vào một dòng log/code diff và dịch sang ngôn ngữ tự nhiên.
        Rất hữu ích khi hiển thị ở 'Góc nhìn Quản lý' (Human View).
        """
        # Đây có thể là nơi gọi một LLM nhỏ (như Haiku/Flash) để dịch,
        # nhưng để real-time và tiết kiệm, ta dùng Regex + Keyword matching.
        
        lower_log = log_text.lower()
        interpreted = ""

        if "created file" in lower_log or "new file:" in lower_log:
            filename = self._extract_filename(log_text)
            interpreted = f"✅ Đã tạo file mới: {filename}"
        elif "modified file" in lower_log or "diff --git" in lower_log:
            filename = self._extract_filename(log_text)
            interpreted = f"⏳ Đang chỉnh sửa và nâng cấp: {filename}"
        elif "npm install" in lower_log or "pip install" in lower_log:
            interpreted = "📦 Đang cài đặt thư viện cần thiết..."
        elif "starting server" in lower_log or "running on http" in lower_log:
            interpreted = "🚀 Đang khởi động máy chủ (server)..."
        elif "error" in lower_log or "exception" in lower_log:
            interpreted = "⚠️ Đang xử lý một lỗi nhỏ trong quá trình build..."
        elif "success" in lower_log or "done" in lower_log:
            interpreted = "✨ Hoàn tất một khối công việc."
            
        if interpreted and (not self.timeline_events or self.timeline_events[-1] != interpreted):
            self.timeline_events.append(interpreted)
            return interpreted
            
        return ""
        
    def _extract_filename(self, text: str) -> str:
        """Trích xuất tên file từ log."""
        match = re.search(r'([a-zA-Z0-9_\-\./\\]+\.[a-zA-Z0-9]+)', text)
        if match:
            return match.group(1).split('/')[-1].split('\\')[-1]
        return "thành phần giao diện"

    def generate_qa_summary(self, original_prompt: str, task_status: str) -> str:
        """
        Sinh ra một báo cáo nghiệm thu giả định dựa trên tiến trình.
        Trong thực tế, có thể dùng LLM để generate nội dung này phong phú hơn.
        """
        if task_status != "done":
            return f"❌ Tác vụ chưa hoàn thành (Trạng thái: {task_status}). Vui lòng kiểm tra lại log."

        summary = (
            "📋 **BÁO CÁO NGHIỆM THU TỪ AGENT QA**\n"
            "---\n"
            "**1. Mục tiêu đã hoàn thành:**\n"
            "Tôi đã tiến hành phân tích và thực thi xong yêu cầu dựa trên tài liệu đính kèm.\n\n"
            "**2. Hướng dẫn kiểm tra trực tiếp:**\n"
            "- Các file chức năng mới đã được lưu.\n"
            "- Nếu là giao diện Web, hãy bật màn hình **Live Preview** để xem trực quan.\n"
            "- Nếu là mã nguồn, bạn có thể chạy thử.\n\n"
            "**3. Bạn muốn làm gì tiếp theo?**\n"
            "(Vui lòng bấm chọn các gợi ý lệnh ở màn hình hoặc chat thêm với tôi)"
        )
        return summary
