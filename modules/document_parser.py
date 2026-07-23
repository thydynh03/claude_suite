import os
import base64
import fnmatch

class DocumentParser:
    """
    Module xử lý việc đọc nội dung của các file được đính kèm vào hệ thống
    (PDF, Code, Text, Image) để feed cho LLM Context.
    """

    # Các thư mục hoặc file mặc định cần bỏ qua khi đính kèm cả một folder
    DEFAULT_IGNORE_PATTERNS = [
        "node_modules", ".git", ".venv", "venv", "__pycache__", 
        "*.exe", "*.dll", "*.so", "*.dylib", "*.pyc", "*.pyo", 
        ".DS_Store", "*.jpg", "*.png", "*.gif", "*.mp4", "*.zip", "*.tar.gz"
    ]

    @staticmethod
    def should_ignore(filepath: str, extra_patterns: list = None) -> bool:
        """Kiểm tra xem file/thư mục có nằm trong danh sách cần bỏ qua không."""
        basename = os.path.basename(filepath)
        patterns = DocumentParser.DEFAULT_IGNORE_PATTERNS
        if extra_patterns:
            patterns.extend(extra_patterns)
        
        for pattern in patterns:
            if fnmatch.fnmatch(basename, pattern):
                return True
        return False

    @staticmethod
    def parse_file(filepath: str) -> str:
        """
        Tự động nhận diện đuôi file và gọi hàm xử lý tương ứng.
        Trả về string nội dung để đưa vào Prompt/Context.
        """
        if not os.path.exists(filepath):
            return f"[ERROR: File not found - {filepath}]"

        ext = os.path.splitext(filepath)[1].lower()
        
        try:
            if ext == ".pdf":
                return DocumentParser.parse_pdf(filepath)
            elif ext in [".png", ".jpg", ".jpeg", ".webp"]:
                # Thông báo cho LLM đây là ảnh đã được đính kèm
                # Giao diện gọi API sẽ sử dụng parse_image_to_base64()
                return f"[IMAGE: {os.path.basename(filepath)} - Nội dung hình ảnh đã được đính kèm vào LLM payload]"
            else:
                return DocumentParser.parse_text(filepath)
        except Exception as e:
            return f"[ERROR parsing {filepath}: {str(e)}]"

    @staticmethod
    def parse_text(filepath: str) -> str:
        """Đọc file text/code thông thường."""
        with open(filepath, "r", encoding="utf-8", errors="replace") as f:
            return f.read()

    @staticmethod
    def parse_pdf(filepath: str) -> str:
        """Sử dụng PyMuPDF để lấy text từ PDF."""
        try:
            import fitz  # PyMuPDF
            doc = fitz.open(filepath)
            text = []
            for page in doc:
                text.append(page.get_text())
            doc.close()
            return "\n".join(text)
        except ImportError:
            return f"[ERROR: PyMuPDF not installed. Cannot parse PDF {filepath}. Please run 'pip install PyMuPDF']"
        except Exception as e:
            return f"[ERROR reading PDF {filepath}: {str(e)}]"

    @staticmethod
    def parse_image_to_base64(filepath: str) -> str:
        """
        Chuyển đổi ảnh sang Base64 Data URI để truyền trực tiếp vào Vision Model.
        Ví dụ kết quả: data:image/png;base64,iVBORw0KGgo...
        """
        if not os.path.exists(filepath):
            return ""
            
        ext = os.path.splitext(filepath)[1].lower()
        mime_type = "image/jpeg"
        if ext == ".png":
            mime_type = "image/png"
        elif ext == ".webp":
            mime_type = "image/webp"

        with open(filepath, "rb") as image_file:
            encoded_string = base64.b64encode(image_file.read()).decode("utf-8")
            return f"data:{mime_type};base64,{encoded_string}"

    @staticmethod
    def scan_folder(folder_path: str) -> dict:
        """
        Quét thư mục, bỏ qua các file/thư mục rác.
        Trả về dict dạng { 'filepath': 'content' }
        """
        result = {}
        for root, dirs, files in os.walk(folder_path):
            # Lọc thư mục rác
            dirs[:] = [d for d in dirs if not DocumentParser.should_ignore(d)]
            
            for file in files:
                if not DocumentParser.should_ignore(file):
                    filepath = os.path.join(root, file)
                    # Bỏ qua các file quá lớn (vd > 5MB) để tránh tràn bộ nhớ
                    if os.path.getsize(filepath) <= 5 * 1024 * 1024: 
                        content = DocumentParser.parse_file(filepath)
                        result[filepath] = content
                    else:
                        result[filepath] = f"[SKIPPED: File larger than 5MB]"
        return result
