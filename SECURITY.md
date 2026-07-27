# Security Policy

[English](#english) · [Tiếng Việt](#tiếng-việt)

---

## English

### Reporting a vulnerability

**Do not open a public issue for a security problem.**

Use GitHub's private vulnerability reporting:
[Report a vulnerability](https://github.com/thydynh03/Agent_Center/security/advisories/new).
It goes only to the maintainers, and you can attach details there safely.

Please include what you would want if you received the report: the version, the
platform, what you did, what happened, and — if you have one — the smallest
reproduction. If you have a proof of concept, say so before attaching it.

You can expect an acknowledgement within a week. This is a small project and
there is no paid response team; an honest estimate is better than a promise
nobody can keep. Fixes ship in a normal tagged release, credited to you unless
you ask otherwise.

**Never paste a token, refresh token, API key or OAuth secret into a report** —
not even a redacted-looking one. If a credential of yours was exposed, revoke it
first at [Google Account permissions](https://myaccount.google.com/permissions),
then tell us how it leaked.

### Supported versions

The latest tagged release is the only supported version. Fixes go onto `master`
and ship in the next tag; there are no long-lived maintenance branches.

### What this app does that you should know about

Agent Center is an agent orchestrator, so several of its normal behaviours are
things a security-minded reader should be told plainly rather than discover.

**Sub-agents run with `--dangerously-skip-permissions`.** They create, edit and
delete files inside the workspace folder you chose, without asking per file.
This is what makes the tool work, and it is why:

- the workspace should be a git repository you have committed, and
- a git snapshot is taken before every task, so each task's changes stay
  recoverable as an uncommitted diff.

Point it at a project you can restore. Do not point it at your home directory.

**Everything that reads or writes user files goes through
`services.EnsureWithinWorkspace`.** A path that escapes the workspace is a
vulnerability, not a feature — report it.

**Credentials at rest.** OAuth refresh tokens are sealed with Windows DPAPI to
the current user, so copying `anti_accounts.json` to another machine or account
does not hand over your Google accounts. On other platforms there is no
equivalent sealing — the file is protected only by its permissions. GCP client
credentials come from environment variables or `gcp_oauth.json` in the data
directory, both gitignored, and are never read from the repository.

**Local listeners.** The app opens three:

| Port | What | Exposure |
|---|---|---|
| `8045` | Google OAuth callback | loopback, only while a sign-in is in progress |
| Inbound webhook | task triggers | **loopback only** — that is the whole security model; see [ARCHITECTURE_DECISIONS.md](docs/ARCHITECTURE_DECISIONS.md) §12 |
| `9111` | claims/adjudication host | reachable on your LAN when running, guarded by a per-session token |

The claims host is the one to think about: it is meant to be joined by teammates'
agents, so it binds beyond loopback. Sessions are token-gated and expire, but do
not run it on a network you do not trust.

**The browser agent drives a real Chrome** with its own persistent profile, kept
separate from your everyday browser precisely so that a site an agent visits
cannot touch your normal cookies. It stays signed in between runs, so treat that
profile as it deserves.

**Sub-agents execute model output.** A prompt that reaches an agent can cause
code to be written and run. Treat task descriptions from an untrusted source the
way you would treat a script from that source.

### Out of scope

- The agent CLIs themselves (`claude`, `agy`) — report those upstream
- Anything requiring an attacker who already has your Windows user session
- The deliberate permissions above, when used as documented

---

## Tiếng Việt

### Báo lỗ hổng bảo mật

**Đừng mở issue công khai cho một vấn đề bảo mật.**

Hãy dùng kênh báo cáo riêng tư của GitHub:
[Report a vulnerability](https://github.com/thydynh03/Agent_Center/security/advisories/new).
Nó chỉ đến tay maintainer, và bạn đính kèm chi tiết ở đó được an toàn.

Xin ghi kèm những thứ chính bạn sẽ muốn có nếu nhận được báo cáo này: phiên bản,
nền tảng, bạn đã làm gì, chuyện gì xảy ra, và nếu có thì cách tái hiện ngắn nhất.
Nếu bạn có mã khai thác mẫu, hãy nói trước khi đính kèm.

Bạn sẽ nhận được phản hồi xác nhận trong vòng một tuần. Đây là dự án nhỏ, không
có đội trực bảo mật; một ước lượng thật thà tốt hơn một lời hứa không ai giữ nổi.
Bản vá đi kèm một bản phát hành có tag bình thường, và ghi công bạn trừ khi bạn
muốn khác.

**Đừng bao giờ dán token, refresh token, API key hay OAuth secret vào báo cáo** —
kể cả bản trông như đã che. Nếu thông tin đăng nhập của bạn bị lộ, hãy thu hồi
trước tại [quyền truy cập tài khoản Google](https://myaccount.google.com/permissions),
rồi mới kể lại nó lộ ra sao.

### Phiên bản được hỗ trợ

Chỉ bản phát hành có tag mới nhất được hỗ trợ. Bản vá vào thẳng `master` và đi
theo tag kế tiếp; không có nhánh bảo trì dài hạn.

### Những gì app này làm mà bạn nên biết

Agent Center là công cụ điều phối agent, nên một số hành vi bình thường của nó là
thứ người quan tâm bảo mật nên được nói thẳng thay vì tự phát hiện.

**Sub-agent chạy với `--dangerously-skip-permissions`.** Chúng tạo, sửa và xoá
file bên trong thư mục workspace bạn đã chọn, không hỏi từng file. Đó chính là
thứ khiến công cụ này hoạt động, và cũng vì thế:

- workspace nên là một repository git mà bạn đã commit, và
- app chụp snapshot git trước mỗi task, để thay đổi của từng task luôn khôi phục
  được dưới dạng diff chưa commit.

Hãy trỏ nó vào một dự án bạn phục hồi được. Đừng trỏ vào thư mục home.

**Mọi đường đọc/ghi file của người dùng đều đi qua
`services.EnsureWithinWorkspace`.** Một đường dẫn thoát được ra ngoài workspace là
lỗ hổng, không phải tính năng — hãy báo cho chúng tôi.

**Thông tin đăng nhập khi lưu trữ.** Refresh token OAuth được niêm phong bằng
Windows DPAPI theo người dùng hiện tại, nên chép `anti_accounts.json` sang máy
hoặc tài khoản khác cũng không lấy được tài khoản Google của bạn. Trên nền tảng
khác không có cơ chế niêm phong tương đương — file chỉ được bảo vệ bằng quyền hệ
thống. Thông tin GCP client đọc từ biến môi trường hoặc `gcp_oauth.json` trong
thư mục dữ liệu, cả hai đều gitignore, và không bao giờ đọc từ repository.

**Các cổng lắng nghe cục bộ.** App mở ba cổng:

| Cổng | Việc gì | Mức phơi bày |
|---|---|---|
| `8045` | callback Google OAuth | loopback, chỉ trong lúc đang đăng nhập |
| Webhook vào | kích hoạt task | **chỉ loopback** — đó là toàn bộ mô hình bảo mật; xem [ARCHITECTURE_DECISIONS.md](docs/ARCHITECTURE_DECISIONS.md) mục 12 |
| `9111` | host phân xử claims | truy cập được trong LAN khi đang chạy, có token theo từng phiên |

Cổng đáng lưu tâm là claims host: nó sinh ra để agent của đồng đội tham gia, nên
lắng nghe ra ngoài loopback. Phiên có token và có hạn, nhưng đừng chạy nó trên
mạng bạn không tin tưởng.

**Browser agent điều khiển một Chrome thật** với profile riêng, tách khỏi trình
duyệt hằng ngày của bạn đúng để một trang mà agent ghé thăm không chạm được vào
cookie thường ngày. Profile đó giữ trạng thái đăng nhập giữa các lần chạy, nên
hãy đối xử với nó tương xứng.

**Sub-agent thực thi output của mô hình.** Một prompt đến được tay agent có thể
khiến code được viết ra và chạy. Hãy đối xử với mô tả task từ nguồn không tin cậy
đúng như cách bạn đối xử với một script từ nguồn đó.

### Ngoài phạm vi

- Bản thân các agent CLI (`claude`, `agy`) — hãy báo lên dự án gốc của chúng
- Bất cứ thứ gì đòi hỏi kẻ tấn công đã chiếm được phiên Windows của bạn
- Các quyền có chủ ý nêu trên, khi được dùng đúng như tài liệu mô tả
