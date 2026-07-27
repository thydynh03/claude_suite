# Plan 2 — Logic hardening cho user mới tải app

Mục tiêu: một người lạ tải installer/portable về một máy Windows sạch (không
Claude CLI, không Gemini CLI, không git, không config, có thể offline) phải
dùng được app mà không gặp lỗi câm, lỗi khó hiểu, hay crash.

## Các lớp rủi ro đang soi

1. **First-run**: data dir trống, DB mới, không `gcp_oauth.json`, không
   `anti_accounts.json`, không roles dir, chưa chọn workspace.
2. **Thiếu binary ngoài**: claude/gemini/git/node vắng mặt — thông báo phải nói
   rõ thiếu gì và cài ở đâu, không được im lặng hay stacktrace.
3. **Offline/mạng chập chờn**: update check, model catalog, key validation,
   claims host — phải fail mềm. (Fonts/icons đã bundle offline — xong.)
4. **Windows đặc thù**: chạy từ Downloads, Program Files từ chối ghi, đường dẫn
   có dấu tiếng Việt/khoảng trắng, OneDrive.
5. **Nil/panic**: service setter tùy chọn chưa gọi, port 9111 đã bị chiếm,
   goroutine leak.

## Danh sách lỗi từ scan (54 finding backend — 10 high, ~26 medium, ~18 low)

High (đang qua vòng phản biện ở workflow wf_e21390d7 trước khi sửa):

1. `updater.go` — bat portable ghi UTF-8 nhưng cmd.exe đọc OEM codepage: đường
   dẫn có dấu tiếng Việt/`%` → swap fail, app thoát không relaunch.
2. `verify.go` — runCmd vứt err: máy không có npm/Go → mọi task fail với report
   RỖNG, không nói lý do.
3. `app.go` — InitDB fail bị nuốt → chạy tiếp với db nil → panic lúc startup,
   không một dòng chẩn đoán (windowsgui nuốt stdout).
4. `resolver.go` — nhận diện Claude **Desktop** (AnthropicClaude) là Claude CLI
   → mọi task treo 10 phút rồi timeout, lặp vô hạn.
5. `claude.go` — cả prompt (kèm context pack) nhét vào argv: vượt 8K của
   cmd.exe (shim .cmd) / 32K của CreateProcess → spawn fail khó hiểu.
6. `claude.go` — workspace không tồn tại → âm thầm chạy agent
   (--dangerously-skip-permissions) ngay tại cwd của app (Downloads!).
7. `notifier.go` — payload không đúng format Slack/Discord → 400 bị nuốt: tính
   năng quảng cáo trong Settings không bao giờ chạy, không thể debug.
8. `webhook.go` — bind mọi interface, không auth: ai cùng LAN POST được task mà
   sub-agent sẽ thực thi (prompt injection → ghi file).
9. `app.go` — RefreshAccountTokens disable VĨNH VIỄN mọi account khi lỗi mạng
   tạm thời (offline lúc import) — chỉ invalid_grant mới đáng disable.
10. `antigravity.go` — không refresh OAuth token hết hạn (~1h) trước khi chạy →
    401 → rotation đánh dấu nhầm account khỏe là rate_limited 24h.

Medium/low: timeout thiếu cho update check/download, sự kiện startup phát
trước khi frontend nghe, config ghi `_ =` không atomic, gcp_oauth.json sai
shape bị coi là thiếu, ID account trùng sau delete-then-add, claims host
Stop/Start giữ port 9111, session TTL 60' đá MCP participant giữa chừng,
SingleInstanceLock thiếu, v.v. — đầy đủ trong `findings-verify-*.md`
(scratchpad phiên) cùng phán quyết CONFIRMED/REFUTED của 4 verifier.

## Quy trình

scan (11 agent) → verify từng finding (phản biện độc lập, bỏ finding không
tái hiện được) → fix theo severity → review lại diff sau fix → test
(`go build/vet/test`, svelte-check, vitest, npm build) → còn lỗi thì lặp.

## Điều kiện nghiệm thu — đã đạt

- Toàn bộ 10 finding high đã sửa; các finding bị bác có lý do ghi lại (ví dụ
  `os.Exit(0)` trong updater: app này vốn không đăng ký OnShutdown nào, nên
  không có cleanup nào bị bỏ qua — finding sai tiền đề).
- Test mới pin đúng các lỗi vừa sửa: codepage + dấu `%` của bat, ID account
  trùng sau delete, peek-vs-use trên pool, nhận diện adapter ảo.
- `go build ./...`, `go vet ./...`, `go test ./backend/...` xanh; CI xanh
  (bao gồm job race detector trên Linux) sau mỗi lần push.
