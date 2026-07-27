# Documentation index

[English](#english) · [Tiếng Việt](#tiếng-việt)

---

## English

### Start here

| Document | What it is for | Language |
|---|---|---|
| [README](../README.md) · [🇻🇳](../README.vi.md) | Install, first run, features, troubleshooting | EN + VI |
| [CONTRIBUTING](../CONTRIBUTING.md) · [🇻🇳](../CONTRIBUTING.vi.md) | Prerequisites, what CI runs, the rules that are not obvious | EN + VI |
| [SECURITY](../SECURITY.md) | Threat model, what the app does that you should know about, how to report | EN + VI |
| [CODE_OF_CONDUCT](../CODE_OF_CONDUCT.md) | How we treat each other | EN + VI |

### Reference

| Document | What it is for | Language |
|---|---|---|
| [ARCHITECTURE_DECISIONS](ARCHITECTURE_DECISIONS.md) | **Read before "simplifying" anything.** 13 decisions that look like unnecessary detours and are load-bearing: `/c` vs `/k`, the `file:` SQLite URL, the dedicated Chrome profile, per-task approval channels, the synchronous webhook bind | EN |
| [CLAUDE.md](../CLAUDE.md) | Orientation for AI coding agents working in this repository, including how to file a claim with a falsifier | EN |

### Internal plans

These are working documents, not user documentation. They record what was
decided and why, in the order it happened, and some of it is already done. They
are kept because the reasoning is worth more than the checklist.

| Document | What it is for | Language |
|---|---|---|
| [PLAN_UI_MINIMAL](PLAN_UI_MINIMAL.md) | The minimal-interface pass: one accent, icons that are icons, and the cascade-layer trap that made all 34 icons render at 24px | VI |
| [PLAN_LOGIC_HARDENING](PLAN_LOGIC_HARDENING.md) | The correctness pass: how the findings were scanned, verified and fixed | VI |
| [UX_BACKLOG_2026-07](UX_BACKLOG_2026-07.md) | The July 2026 defect and upgrade backlog, page by page | VI |
| [MEMORY_CODEGRAPH_PLAN](MEMORY_CODEGRAPH_PLAN.md) | Design for the project map and persistent agent memory | VI |

### A note on languages

The application interface ships in Vietnamese and English. The user-facing
documents above are maintained in both; the reference and planning documents are
in whichever language they were written, because translating a working document
tends to leave two versions that disagree.

If you change how the app is used, update **both** language versions of the
user-facing docs in the same pull request.

---

## Tiếng Việt

### Bắt đầu từ đây

| Tài liệu | Dùng để làm gì | Ngôn ngữ |
|---|---|---|
| [README](../README.vi.md) · [🇬🇧](../README.md) | Cài đặt, chạy lần đầu, tính năng, xử lý sự cố | VI + EN |
| [CONTRIBUTING](../CONTRIBUTING.vi.md) · [🇬🇧](../CONTRIBUTING.md) | Yêu cầu môi trường, CI chạy gì, những quy tắc không hiển nhiên | VI + EN |
| [SECURITY](../SECURITY.md) | Mô hình mối đe doạ, những gì app làm mà bạn nên biết, cách báo lỗ hổng | EN + VI |
| [CODE_OF_CONDUCT](../CODE_OF_CONDUCT.md) | Cách chúng ta đối xử với nhau | EN + VI |

### Tra cứu

| Tài liệu | Dùng để làm gì | Ngôn ngữ |
|---|---|---|
| [ARCHITECTURE_DECISIONS](ARCHITECTURE_DECISIONS.md) | **Đọc trước khi "đơn giản hoá" bất cứ thứ gì.** 13 quyết định trông như đường vòng thừa thãi nhưng đang gánh cả hệ thống: `/c` với `/k`, URL `file:` cho SQLite, profile Chrome riêng, kênh phê duyệt theo từng task, việc bind webhook đồng bộ | EN |
| [CLAUDE.md](../CLAUDE.md) | Định hướng cho agent AI làm việc trong repo này, gồm cả cách nộp claim kèm falsifier | EN |

### Kế hoạch nội bộ

Đây là tài liệu làm việc, không phải tài liệu cho người dùng. Chúng ghi lại điều
gì đã được quyết và vì sao, theo đúng thứ tự diễn ra, và một phần trong đó đã
hoàn thành. Chúng được giữ lại vì phần lý lẽ đáng giá hơn phần danh sách gạch đầu
dòng.

| Tài liệu | Dùng để làm gì | Ngôn ngữ |
|---|---|---|
| [PLAN_UI_MINIMAL](PLAN_UI_MINIMAL.md) | Đợt làm giao diện tối giản: một màu nhấn, icon ra icon, và cái bẫy cascade layer khiến cả 34 icon render ở 24px | VI |
| [PLAN_LOGIC_HARDENING](PLAN_LOGIC_HARDENING.md) | Đợt siết tính đúng đắn: các finding được quét, kiểm chứng và sửa ra sao | VI |
| [UX_BACKLOG_2026-07](UX_BACKLOG_2026-07.md) | Danh sách lỗi và nâng cấp tháng 7/2026, theo từng trang | VI |
| [MEMORY_CODEGRAPH_PLAN](MEMORY_CODEGRAPH_PLAN.md) | Thiết kế cho project map và bộ nhớ bền vững của agent | VI |

### Ghi chú về ngôn ngữ

Giao diện ứng dụng có sẵn tiếng Việt và tiếng Anh. Các tài liệu dành cho người
dùng ở trên được duy trì bằng cả hai thứ tiếng; tài liệu tra cứu và kế hoạch giữ
nguyên ngôn ngữ nó được viết ra, vì dịch một tài liệu đang làm việc thường để lại
hai bản mâu thuẫn nhau.

Nếu bạn thay đổi cách dùng app, hãy cập nhật **cả hai** bản ngôn ngữ của tài liệu
người dùng trong cùng một pull request.
