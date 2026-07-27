# Đóng góp cho Claude Suite

[English](CONTRIBUTING.md) · **Tiếng Việt**

Cảm ơn bạn đã muốn giúp. Trang này nói về cách dựng môi trường chạy dự án, một vài
quy tắc không hiển nhiên khi chỉ đọc code, và những gì CI sẽ kiểm tra.

Trước khi sửa thứ gì trông có vẻ thừa, hãy đọc
[`docs/ARCHITECTURE_DECISIONS.md`](docs/ARCHITECTURE_DECISIONS.md) — nó liệt kê những
quyết định đã từng khiến ai đó mất cả buổi để gỡ.

Khi tham gia, bạn đồng ý với [Quy tắc ứng xử](CODE_OF_CONDUCT.md).

---

## Bạn có thể giúp bằng cách nào

| Bạn muốn… | Bắt đầu từ đây |
|---|---|
| Báo lỗi | [Mở một bug report](https://github.com/thydynh03/claude_suite/issues/new?template=bug_report.yml) — kèm phiên bản trên thanh tiêu đề và file log trong thư mục dữ liệu |
| Đề xuất tính năng | [Mở một feature request](https://github.com/thydynh03/claude_suite/issues/new?template=feature_request.yml) — mô tả vấn đề trước, giải pháp sau |
| Sửa việc nhỏ | Bất kỳ issue nào gắn nhãn [`good first issue`](https://github.com/thydynh03/claude_suite/labels/good%20first%20issue) |
| Dịch thuật | `frontend/src/lib/stores/i18n.ts` chứa toàn bộ chuỗi. Giữ nhãn ngắn — thanh điều hướng rộng 240px và có test làm vỡ build khi một nhãn không còn vừa |
| Cải thiện tài liệu | Mọi sửa đổi đều được hoan nghênh, kể cả lỗi chính tả. Hai bản ngôn ngữ nên đi cùng nhau |

Bạn không cần xin phép trước khi mở pull request. Với thứ đủ lớn để bạn thấy tiếc nếu
bị từ chối, hãy mở issue trước để chốt hướng làm cho rẻ.

---

## Yêu cầu môi trường

| Công cụ | Phiên bản | Ghi chú |
|---|---|---|
| Go | như ghim trong `go.mod` | dòng toolchain là nguồn quyết định |
| Node.js | 20.x | cho frontend Svelte |
| Wails CLI | v2.13.0 | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0` |

SQLite dùng `modernc.org/sqlite` — driver thuần Go, nên **không cần gcc** để build hay
test; chỉ race detector mới cần CGO, và nó chạy trên Linux trong CI.

---

## Chạy dự án

Có hai giao diện trên một backend. Cả hai đọc cùng một database.

```bash
# App desktop (Wails + Svelte), hot reload
wails dev

# Giao diện terminal — mặc định chỉ đọc: không migration, không ghi
go run ./cmd/claude-suite-tui --db /path/to/agent_manager.db

# Giao diện terminal có quyền sửa task và điều phối
go run ./cmd/claude-suite-tui --db /path/to/agent_manager.db --write
```

Build bản phát hành:

```bash
wails build -platform windows/amd64 -clean   # -> build/bin/ClaudeSuite.exe
```

Trong lúc phát triển, hãy trỏ app vào một **workspace vứt đi được**. Sub-agent chạy
với `--dangerously-skip-permissions` và sẽ sửa file thật.

---

## CI chạy những gì

Chạy đoạn này ở máy bạn trước khi mở pull request — đây đúng là những gì
`.github/workflows/ci.yml` làm:

```bash
go build ./backend/... ./cmd/...
go vet ./backend/... ./cmd/...
go test ./backend/... ./cmd/...
cd frontend && npm ci && npm run check && npm run test && npm run build
```

Test frontend chạy trên Vitest. `npm run test:watch` chạy lại khi bạn sửa. Store và
tiện ích là TypeScript thuần, không cần DOM; test component dùng
`@testing-library/svelte` trên jsdom, cấu hình trong `vitest.config.ts` — tách khỏi
`vite.config.ts` một cách có chủ ý, để công cụ test không thể làm đổi hành vi của bản
build production.

Hai điều trong danh sách lệnh trên là cố ý:

- **Thư mục gốc bị loại trừ.** `main.go` có `//go:embed all:frontend/dist`, thứ chưa
  tồn tại cho tới khi frontend được build. `./cmd/...` thì *có* nằm trong danh sách —
  binary TUI đã từng không được CI biên dịch cho tới khi có người phát hiện. Thư mục
  gốc được phủ bởi job **Desktop app build** riêng, chạy `wails build` thật; job đó
  là thứ duy nhất đứng giữa một `app.go` hỏng và ngày phát hành, và nó cũng fail nếu
  `frontend/wailsjs` đã lệch khỏi mã Go.
- **Race detector chạy trên Linux trong CI**, không phải trên máy bạn. `-race` cần
  CGO, mà bản checkout trên Windows thường không có gcc. Nếu bạn động vào goroutine
  của orchestrator, cứ để CI kiểm.

`codeql.yml` chạy phân tích tĩnh ở mỗi lần push, còn `release.yml` build bộ cài khi
một tag `v*` được đẩy lên.

---

## Những quy tắc không hiển nhiên

### 1. Một năng lực phải có cùng tên method ở cả hai giao diện

`app.go` (Wails) và `backend/tui/task_actions.go` (TUI) là hai adapter song song trên
cùng bộ service. Đã từng có tám năng lực tồn tại dưới hai cái tên —
`GetGitStatus`/`GitStatus`, `ExportKanbanReport`/`ExportReport`, và sáu cái nữa — mà
review không bắt được cái nào.

`backend/contract` giờ làm vỡ build khi TUI khai báo một năng lực mà bản Wails không
có dưới cùng tên. Nếu một method thật sự chỉ thuộc về một giao diện, hãy thêm nó vào
`tuiOnlyMethods` **kèm lý do**.

Chữ ký hàm được phép khác nhau: Wails trả giá trị về JavaScript, TUI thì thay đổi
model Bubble Tea của nó. Chỉ tên là bị ràng buộc.

### 2. Wails binding là code sinh tự động — đừng bao giờ sửa tay

Sau khi đổi một method của `App`, hoặc bất kỳ struct nào đi qua ranh giới đó:

```bash
wails generate module
```

Lệnh này ghi lại `frontend/wailsjs/`. Bản sửa tay đã lệch khỏi `app.go` hai lần và
làm vỡ build cả hai lần. Lưu ý: một tiến trình `wails dev` đang chạy có thể xoá các
file này trong lúc build lại — hãy sinh lại chứ đừng khôi phục bản cũ.

### 3. Đừng bao giờ so khớp từ khoá ngắn bằng `strings.Contains`

Việc định tuyến agent đã hỏng hai lần vì so khớp chuỗi con: `"DATABASE"` khớp vai trò
`BA`, và `"latest"` khớp từ khoá `test` rồi đẩy task sang agent QA. Hãy dùng ranh giới
từ — xem `orchestrator.keywordMatch` và `models.keywordTags`.

### 4. Bí mật không bao giờ nằm trong mã nguồn

Thông tin GCP OAuth được đọc từ `CLAUDE_SUITE_GCP_CLIENT_ID` /
`CLAUDE_SUITE_GCP_CLIENT_SECRET`, hoặc từ `gcp_oauth.json` trong thư mục dữ liệu. Cả
hai đều nằm trong gitignore. Đừng bao giờ dán token vào một issue — xem
[SECURITY.md](SECURITY.md).

### 5. Sub-agent ghi thẳng vào workspace

Chúng chạy với `--dangerously-skip-permissions`. Orchestrator chụp snapshot git trước
mỗi task để thay đổi của riêng task đó nằm lại dưới dạng diff chưa commit. Mọi đường
đi mới có đọc hoặc ghi file của người dùng đều phải qua
`services.EnsureWithinWorkspace`.

### 6. State dùng chung đi qua goroutine nhiều hơn vẻ ngoài của nó

Các CLI runner là instance dùng chung và mỗi task chạy trong goroutine riêng. Một
phép gán field bình thường trong runner đã là data race, kể cả khi nó trông chỉ như
một dòng cache — xem `backend/cli/resolved_path.go` để biết hình dạng đúng của cách
sửa.

### 7. CSS của thư viện import từ JS nằm ngoài layer, và thắng mọi utility class

Một khai báo ngoài layer thắng khai báo trong layer ở cùng độ đặc hiệu, nên stylesheet
của package import trong `main.ts` âm thầm đè lên Tailwind. Hãy import nó từ `app.css`
với `layer(base)`, và kiểm chứng bằng `getComputedStyle` trên app đang chạy chứ không
bằng mắt — lỗi này vô hình trong ảnh chụp màn hình.

---

## Mọi thứ nằm ở đâu

| Đường dẫn | Nội dung |
|---|---|
| `app.go` | các method Wails — bề mặt API của app desktop |
| `backend/orchestrator/` | worker pool, điều phối, phê duyệt và retry theo từng task |
| `backend/cli/` | runner cho Claude và Antigravity CLI |
| `backend/services/` | git, browser agent, webhook, xuất báo cáo, lập lịch |
| `backend/claims/` | host phân xử, bộ chạy falsifier và MCP server |
| `backend/database/` | repository SQLite và migration |
| `backend/tui/` | giao diện terminal (các file `model` / `update` / `commands` / `view`) |
| `backend/contract/` | kiểm tra nhất quán giữa hai giao diện |
| `frontend/src/` | UI desktop Svelte 5 (`*.test.ts` nằm cạnh thứ nó kiểm) |
| `cmd/claude-suite-tui/` | điểm vào của TUI |
| `cmd/claude-suite-claim/` | CLI để nộp claim |

---

## Commit và pull request

Message commit theo [Conventional Commits](https://www.conventionalcommits.org/):
`fix:`, `feat:`, `refactor:`, `test:`, `docs:`, `chore:`, có thể kèm phạm vi như
`fix(ui):`. Dòng tiêu đề nói cái gì đã đổi; phần thân nói **vì sao**, và trước đó nó
sai thế nào. Một message chỉ kể lại diff thì không đáng cái dòng nó chiếm.

Chính app cũng viết nháp được một message từ diff, trong panel Source Control.

### Danh sách kiểm trước khi mở pull request

- [ ] `go build`, `go vet` và `go test` xanh cho `./backend/... ./cmd/...`
- [ ] `npm run check`, `npm run test` và `npm run build` xanh trong `frontend/`
- [ ] Đã chạy `wails generate module` nếu có đổi method của `App` hoặc struct dùng chung
- [ ] Năng lực mới dùng cùng tên method ở cả hai giao diện
- [ ] Thay đổi hành vi đi kèm một test mà nếu bỏ thay đổi thì test đó fail
- [ ] Chuỗi hiển thị cho người dùng có đủ cả `vi` lẫn `en`
- [ ] Tài liệu được cập nhật ở cả hai ngôn ngữ nếu cách dùng app thay đổi
- [ ] Không có thông tin đăng nhập, token hay đường dẫn tuyệt đối của máy trong diff

---

## Báo một lỗi bạn tìm thấy khi review

Nếu bạn là một agent — hoặc một người đang dùng agent — repository này có sẵn cách để
phát biểu một phát hiện sao cho nó có thể bị chứng minh là sai, thay vì viết thẳng vào
comment của PR:

```bash
claude-suite-claim --checks                    # những thứ bạn được phép trỏ vào
claude-suite-claim --host ws://HOST:9111 --session ID --token T \
  --author you/your-agent --provider claude \
  --subject "backend/cli/process_windows.go:17" \
  --assert  "cmd.Wait() never returns when the console is visible" \
  --falsify "console-window-hidden"
```

`--falsify` gọi tên một mục trong `.claude-suite/checks.json`, và **một falsifier pass
khi claim của bạn sai** — nên check thất bại mới là thứ xác nhận lỗi. Bỏ nó đi thì
claim được giữ lại như một ý kiến, và ý kiến thì không chặn được merge. Chi tiết đầy
đủ, gồm cả cách kết nối qua MCP, nằm trong [CLAUDE.md](CLAUDE.md).
