# Kế hoạch sửa lỗi & nâng cấp — tháng 7/2026

Tài liệu này ghi lại **19 hạng mục** người dùng báo, kèm **trạng thái thực tế của source
code hiện tại** (đã đọc code để kiểm chứng, không đoán). Nhiều nút bị báo là "không chạy"
thực ra *có chạy* nhưng không phản hồi gì trên UI — phải phân biệt hai loại đó trước khi
sửa, vì cách sửa hoàn toàn khác nhau.

Ký hiệu trạng thái:

| Ký hiệu | Nghĩa |
| --- | --- |
| 🔇 **Silent** | Backend chạy đúng, UI không phản hồi → chỉ cần thêm feedback |
| 🕳️ **Rỗng** | Không có backend thật, đang trả dữ liệu bịa/hardcode |
| 🐛 **Lỗi** | Có code nhưng sai |
| ➕ **Thiếu** | Chưa tồn tại, phải xây mới |

---

## 0. Nguyên nhân gốc của "cảm giác app bị đơ"

Đây là phát hiện quan trọng nhất và nó giải thích **6 trong 19 hạng mục**.

Dự án đã có sẵn hệ thống toast hoàn chỉnh:

- `frontend/src/lib/stores/appState.ts:44` — store `toasts` + hàm `addToast(message, level)`
- `frontend/src/components/ui/ToastHost.svelte` — đã được mount trong `App.svelte`
- `frontend/src/components/pages/GitPanel.svelte:79` — **đang dùng đúng cách**, đây là mẫu

Nhưng hầu hết các trang khác lại gọi `addLog(...)` thay vì `addToast(...)`. `addLog` đẩy chữ
vào panel Live Log — nằm ở tab khác, thường bị thu gọn. Người dùng bấm nút, backend chạy
xong, mà màn hình không đổi gì cả → kết luận "nút chết".

**Quy tắc từ nay:** mỗi hành động do người dùng bấm phải có đủ ba trạng thái nhìn thấy được:

1. **Đang chạy** — nút disabled + spinner, không cho bấm hai lần
2. **Kết quả** — `addToast` thành công *hoặc* thất bại (kể cả khi backend không trả lỗi)
3. **Dữ liệu mới** — reload lại phần bị ảnh hưởng

`addLog` vẫn giữ, nhưng chỉ để lưu vết, **không được là phản hồi duy nhất**.

> Sẽ viết một test chặn hồi quy: quét các `.svelte`, nếu một handler `on:click` gọi
> `AppBindings.*` mà không có `addToast` trong cùng hàm thì fail. Rẻ hơn nhiều so với
> phát hiện lại bằng mắt sau mỗi release.

---

## 1. Pool tài khoản Google OAuth

### 1.1 🕳️ Hạn mức model là số bịa — không phải "chưa lấy được"

`backend/cli/antigravity.go:50` — hàm `defaultQuotas()` trả về **hằng số cứng**:

```go
{Name: "Gemini 3.1 Pro (High)", ResetTime: "0h 0m", UsagePct: 53, Status: "ok"},
```

Không có một lời gọi mạng nào để lấy quota. Con số 53% và 8% mà UI đang hiện là chữ viết sẵn
trong code, giống hệt nhau cho mọi tài khoản. `ResetTime` cũng vậy.

Việc phải làm:
- Thêm `backend/cli/quota.go`: gọi endpoint quota của Antigravity/Gemini bằng access token
  của từng tài khoản, parse ra hạn mức thật.
- Cache theo tài khoản kèm `fetched_at`; UI hiện "cập nhật 3 phút trước".
- Khi gọi hỏng: hiện **"chưa lấy được"** kèm lý do, **tuyệt đối không fallback về số bịa** —
  số bịa nguy hiểm hơn ô trống vì người dùng tin nó.
- Chỉ sau khi có số thật mới nối vào việc lên lịch theo giờ reset quota (đang nằm trong
  backlog cũ).

### 1.2 🕳️ Không phân biệt được tài khoản PRO

`Tier` được gán cứng `"PRO"` ở `antigravity.go:149,172,567`, còn tài khoản import bằng
refresh token thì mặc định `FREE` — nên bộ lọc `PRO (0) / ULTRA (0)` luôn bằng 0. Tier chưa
bao giờ được hỏi từ Google.

Việc phải làm: đọc tier thật từ cùng response quota ở 1.1; badge FREE/PRO/ULTRA lấy từ đó;
bộ lọc đếm theo dữ liệu thật.

### 1.3 🔇 Nút "Làm Nóng (Warmup)" và "Làm Mới"

`OAuthPoolDashboard.svelte:176` gọi `WarmupAntiAccountKeys()` rồi chỉ `addLog`.

Ngoài ra `WarmupKeys()` (`antigravity.go:227`) **không hề gọi mạng** — nó chỉ đặt lại
`Status = "active"` và `LastUsed = now` trong bộ nhớ. Đặt tên "làm nóng" là gây hiểu nhầm.

Việc phải làm:
- Đổi hành vi: warmup thật = refresh access token + gọi thử quota cho từng tài khoản.
- Trả về `{ok, failed, errors[]}` thay vì không trả gì.
- UI: progress "đang làm nóng 4/11", kết thúc toast **"9 thành công, 2 lỗi"** kèm nút xem chi
  tiết. Không được báo "thành công" khi có tài khoản lỗi.
- Nút "Làm Mới": disabled + icon xoay trong lúc chạy (biến `isLoading` đã có sẵn, chỉ chưa
  gắn vào toast).

### 1.4 🔇 "Lưu Cấu Hình" GCP credentials

`OAuthPoolDashboard.svelte:87` — gọi `SaveGCPOAuthCredentials` rồi im lặng.

Việc phải làm: toast thành công/thất bại; sau khi lưu thì **ẩn ngay banner cảnh báo vàng** và
bật lại nút "Đăng Nhập Google" — hiện tại banner vẫn nằm đó sau khi lưu, khiến người dùng
tưởng lưu hỏng.

### 1.5 ➕ Cấu hình sẵn credentials cho người dùng cuối

Người dùng hỏi: *"login gg thiếu key hãy gắn vào"*.

Không thể commit Client ID/Secret vào repo — nó sẽ nằm trong mọi bản cài, ai cũng rút ra
được, và Google sẽ thu hồi khi phát hiện. Đây là lý do `CLAUDE.md` cấm hardcode credentials
và cũng là lý do secret cũ đã phải xoay.

Cách làm đúng, theo thứ tự ưu tiên:

1. **Wizard trong app** (nên làm): một màn hình hướng dẫn 4 bước — mở Google Cloud Console →
   tạo OAuth Client dạng *Desktop app* → dán Client ID/Secret → app tự lưu vào
   `gcp_oauth.json` trong thư mục dữ liệu. Kèm nút mở sẵn đúng trang Console và nút copy
   redirect URI. Mỗi người dùng có credential riêng, không ai chia sẻ hạn mức của ai.
2. **Nhập file** `client_secret_*.json` tải thẳng từ Console — parse luôn, đỡ phải copy tay.
3. Với bản dành cho nội bộ nhóm: đặt biến môi trường `CLAUDE_SUITE_GCP_CLIENT_ID` /
   `_CLIENT_SECRET` khi cài, không đưa vào git.

### 1.6 🐛 Checkbox trong bảng tài khoản không có tác dụng

`OAuthPoolDashboard.svelte:357,370` — thẻ `<input type="checkbox">` **không có `bind:`**, và
biến `selectedIds` (dòng 35) khai báo rồi không ai dùng. Checkbox ở header cũng không chọn
tất cả.

Việc phải làm: bind vào `selectedIds`, checkbox header = chọn/bỏ tất cả (có trạng thái
indeterminate), và hiện thanh hành động hàng loạt khi có ít nhất 1 dòng được chọn: **Bật /
Tắt / Làm nóng / Xoá / Kiểm tra quota**. Xoá hàng loạt phải hỏi xác nhận kèm số lượng.

---

## 2. Task Board

### 2.1 🐛 "Export Report" — chạy nhưng thông báo sai

`TaskBoardPage.svelte:329`:

```svelte
const file = await AppBindings.ExportKanbanReport();
addLog(`Exported report to ${file}`, 'SUCCESS');
```

`app.go:549` trả về **nội dung markdown**, không phải đường dẫn. Nên dòng log thành
"Exported report to # Báo cáo…" — cả một file markdown nhét vào một dòng log. File thật *có*
được ghi ra workspace (thấy `ClaudeSuite_Kanban_Report_*.md` trong ảnh chụp Code Studio),
nhưng người dùng không hề biết nó nằm ở đâu.

Việc phải làm: đổi `ExportKanbanReport` trả `(mdPath, htmlPath string, err error)`; UI toast
"Đã xuất báo cáo" + nút **Mở thư mục** và **Mở file**. Khi bảng rỗng thì báo "không có task
nào để xuất" thay vì xuất file trống.

### 2.2 🐛 "Execute Plan" — orchestrator chạy nhưng không có gì để giao

`TaskBoardPage.svelte:151` gọi `StartOrchestrator()` và chỉ `addLog`. Bản thân lời gọi hoạt
động, nhưng có ba chỗ nuốt mất thông tin:

1. `Orchestrator.Start()` (`orchestrator.go:156`) — nếu đang chạy rồi thì **return im lặng**.
   Bấm lần hai không có gì xảy ra và không ai nói gì.
2. `NextDispatchable()` (`task_repo.go:181`) chỉ lấy task ở trạng thái `backlog`/`queued`
   **và có toàn bộ `DependsOn` đã `done`**. Trong ảnh, task E2E duy nhất trong Backlog phụ
   thuộc vào các task đang `Failed` → vĩnh viễn không được giao, **và UI không nói lý do**.
   Đúng như báo cáo: bấm Execute Plan xong không thấy gì chạy.
3. Không có phản hồi khi orchestrator chạy nhưng backlog rỗng.

Việc phải làm:
- `StartOrchestrator` trả rõ trạng thái: `already_running` / `started` / `nothing_to_do`.
- Thẻ task bị chặn: badge **"Chờ: [ARCH] …"** + tooltip liệt kê task còn thiếu; nếu phụ thuộc
  vào task `failed` thì badge đỏ **"Bị chặn bởi task lỗi"** kèm nút *Retry task đó*.
- Toast khi bấm: "Đã khởi động — 3 task sẵn sàng" hoặc "Không có task nào chạy được: 1 task
  đang chờ 2 task lỗi".
- Cột Backlog hiện số task **sẵn sàng / bị chặn** riêng.

### 2.3 🔇 Nút "Tải lại"

Gọi `loadTasks()` xong không đổi gì trên màn hình vì dữ liệu thường giống hệt → trông như
chết. Thêm trạng thái xoay + toast "Đã tải lại — 7 task" (có số lượng thì mới tin là nó chạy).

---

## 3. Code Studio → IDE thật

`CodeStudioPage.svelte` (954 dòng) đang là `<textarea>` + cột số dòng tự vẽ
(dòng 66, 176). Không có tô màu cú pháp, không autocomplete, không gấp code — đúng như nhận
xét "chưa giống IDE".

Việc phải làm, theo thứ tự:

1. **Thay bằng CodeMirror 6** (không phải Monaco: Monaco ~5MB và cần web worker, nặng cho
   WebView2; CodeMirror 6 nhẹ hơn nhiều và đủ dùng). Có sẵn: tô màu cú pháp, gấp code, tìm
   kiếm, multi-cursor, minimap qua plugin.
2. **Cây thư mục thật** — hiện đang là danh sách phẳng 96 file. Đổi sang cây có thư mục
   đóng/mở, icon theo loại file, tìm kiếm mờ (fuzzy) kiểu Ctrl+P.
3. **Tab nhiều file** + đánh dấu chấm khi chưa lưu, Ctrl+S lưu, cảnh báo khi đóng tab bẩn.
4. **Diff xem trước** khi agent sửa file — dùng lại `GetWorkspaceDiff` đã có.
5. **Terminal tích hợp** ở panel dưới, chạy lệnh trong workspace.
6. Giữ nguyên panel AI bên phải nhưng nối vào QuickCLI (xem mục 6).

Đây là hạng mục tốn công nhất — nên tách thành PR riêng, làm sau các mục 🔇 vốn rẻ và nhiều
tác động.

---

## 4. Sắp xếp lại thanh điều hướng trái

Thứ tự hiện tại (`SideNavBar.svelte`): cockpit → kanban → editor → settings → office →
scheduler → claims → browser → git → roles → docs → support.

Vấn đề: **Settings nằm thứ 4**, chen giữa nhóm làm việc; 3D Office (chủ yếu để xem) nằm trên
cả Scheduler và Git; Roles nằm tách khỏi Settings dù cùng chức năng quản trị.

Thứ tự đề xuất, chia nhóm có tiêu đề:

```
LÀM VIỆC       Cockpit · Task Board · Code Studio · Git
TỰ ĐỘNG HOÁ    Orchestrator/Scheduler · Browser Agent · Claims
GIÁM SÁT       3D Office · Reports
CẤU HÌNH       Agent Roles · Settings · OAuth Pool
                                        Docs · Support (đáy)
```

Kèm theo: nhớ tab cuối cùng khi mở lại app, và phím tắt `Ctrl+1..9`.

---

## 5. Form tạo Agent

`SettingsPage.svelte:395` — chỉ có **hai ô**: tên và vai trò. Các trường còn lại bị gán bừa
(`settings.svelte:176` tự sinh system prompt `"You are {name}, {role}."`), trong khi
`models.Agent` có tới 8 trường người dùng nên được đặt: `Provider`, `Model`, `System`,
`Icon`, `TokenLimit`, `Notes`…

Việc phải làm: đổi thành modal đầy đủ, chia nhóm:
- **Danh tính**: tên, vai trò, icon (bảng chọn, không gõ tay), màu
- **Bộ máy**: provider (claude_cli / anti_cli), model (danh sách theo provider), token limit
- **Hành vi**: system prompt (ô lớn, có mẫu dựng sẵn theo vai trò), từ khoá để khớp task
- **Nâng cao**: ghi chú, số lần thử lại tối đa

Có xem trước prompt, kiểm tra trùng tên, và nút "Nhân bản từ agent có sẵn".

---

## 6. QuickCLI

Thực tế QuickCLI **đã được nối** ở ba nơi: `CockpitPage.svelte:100`,
`CodeStudioPage.svelte:297`, `SettingsPage.svelte:245`. Ô prompt ở Cockpit gọi
`RunQuickCLI(promptInput, selectedModel, '', selectedFiles)` và in kết quả ra `cliOutput`.

Vậy vấn đề không phải "chưa tích hợp" mà là **kết quả trả về một cục sau khi chạy xong**:
không stream, không thấy tool call, không huỷ được. Với prompt chạy vài phút thì cảm giác
đúng là "không có tác dụng gì".

Việc phải làm:
- Stream output theo thời gian thực qua sự kiện `log_entry` (hạ tầng đã có, `claude.go` đã
  parse `stream-json`) thay vì chờ hàm trả về.
- Nút **Dừng** khi đang chạy.
- Hiện tool call / file bị sửa như trong Task Inspector.
- Gộp bản sao ở Settings vào Cockpit — một chỗ duy nhất.

---

## 7. Webhook & MCP

`SettingsPage.svelte:717` hiện chỉ có **một ô text** `mcp_connection_string`, lưu vào
`models.IntegrationsConfig.MCPConnectionString`. Không có danh sách, không có kiểm tra kết nối.

Việc phải làm:
- **Danh mục MCP có sẵn**: danh sách đóng gói kèm app (filesystem, git, sqlite, fetch,
  memory, github…) — mỗi mục có mô tả, lệnh cài, trường cấu hình. Bấm là thêm.
- **Tìm trên web**: ô tìm kiếm gọi registry MCP để thêm server ngoài danh mục. Server tải từ
  Internet **phải hỏi xác nhận** trước khi bật, kèm cảnh báo rõ ràng: MCP server chạy được
  lệnh trên máy người dùng.
- **Tự thêm (customize)**: form nhập command/args/env hoặc URL, có nút **Kiểm tra kết nối**
  trả về danh sách tool mà server đó cung cấp — không có nút này thì cấu hình sai chỉ lộ ra
  lúc đang chạy task.
- Bảng quản lý: bật/tắt từng server, xem log, xoá.

---

## 8. Văn phòng 3D

`VirtualOffice3D.svelte` (936 dòng, three.js). Hai vấn đề, đúng như mô tả:

1. **Nhân vật cứng** — `createCharacterAvatar` dựng bằng khối hình học cơ bản, không có
   xương, không có animation đi lại.
2. **Kịch bản dồn cục** — các hàm ở dòng 128–177 gán cùng một `targetX/targetZ` cho cả nhóm
   rồi đổi `bubbleText`. Nên tất cả agent tụ về một chỗ và chỉ hiện bong bóng chat, không
   giống quy trình công ty.

Việc phải làm:
- **Nhân vật**: dùng mô hình có sẵn rig + animation (idle / walk / type / talk) — ví dụ bộ
  nhân vật low-poly kèm animation, nạp bằng GLTF. Rẻ hơn nhiều so với tự làm skeleton.
- **Gắn chuyển động vào task thật**: mỗi agent có bàn riêng; khi task chuyển sang `running`,
  agent đi từ bàn → khu vực tương ứng loại task (Dev zone / QA zone / Meeting room), ngồi
  xuống, gõ máy. Task `done` → đứng dậy về bàn. Task `failed` → animation lắc đầu + biểu
  tượng đỏ trên đầu.
- **Quy trình có thứ tự**: BA họp trước → Architect vẽ bảng → Dev về bàn code → QA sang bàn
  Dev review → DevOps ra khu deploy. Chạy theo DAG phụ thuộc thật của task, không phải kịch
  bản cố định.
- Bong bóng chat chỉ hiện khi thật sự có hai agent tương tác (ví dụ task này phụ thuộc task
  kia), không hiện liên tục.

---

## 9. Hệ thống phân xử (Claims)

### 9.1 Trả lời câu hỏi: `--author YOU/your-agent` là **hardcode**

`app.go:1374` ghép chuỗi lệnh, và phần author là chữ cố định:

```go
`--author YOU/your-agent --provider claude `+
```

`cmd/claude-suite-claim/main.go:40` cũng để mặc định rỗng. Không có chỗ nào đọc tên máy hay
tên người dùng. Đây là chỗ giữ chỗ, người nhận phải tự sửa tay — và họ thường quên, nên các
claim gửi lên đều mang tên `YOU/your-agent`.

Việc phải làm: mặc định lấy `os.UserHomeDir` / `os.Hostname` → `an@may-cua-an/claude-code`,
vẫn cho phép ghi đè bằng cờ `--author`. Lệnh copy ra sẽ chạy được ngay mà không cần sửa.

### 9.1b Trả lời: đường dẫn `C:\Program Files\thydynh03\Claude Suite\claude-suite-claim.exe`
**là tự lấy theo máy, không hardcode** — nhưng chữ `thydynh03` thì có vấn đề riêng

Hai phần của chuỗi lệnh có nguồn gốc khác hẳn nhau, nên trả lời tách ra:

**Phần đường dẫn — lấy động lúc chạy.** `app.go:1433`, hàm `claimToolCommand()`:

```go
exe, err := os.Executable()          // đường dẫn thật của app đang chạy
if err == nil {
    beside := filepath.Join(filepath.Dir(exe), toolName)
    if info, statErr := os.Stat(beside); statErr == nil && !info.IsDir() {
        return `"` + beside + `"`    // có bọc nháy vì "Program Files" có dấu cách
    }
}
return "claude-suite-claim"          // dự phòng: dựa vào PATH
```

Nó hỏi hệ điều hành app đang nằm ở đâu, rồi tìm `claude-suite-claim.exe` **ngay cạnh**. Nên:

- Người dùng cài vào `D:\Apps\ClaudeSuite` → lệnh sinh ra trỏ đúng `D:\Apps\ClaudeSuite\...`
- Cài theo kiểu user (`%LOCALAPPDATA%\Programs\...`) → cũng đúng
- Chạy bản portable `ClaudeSuite.exe` từ USB mà không có công cụ đi kèm → `os.Stat` thất
  bại, rơi về `claude-suite-claim` trần, tức là phụ thuộc PATH. Trường hợp này lệnh sẽ báo
  "not found" — chính là lỗi mà agent bên máy khác từng gặp, và là lý do v2.14.1 đóng gói
  công cụ vào bộ cài.

Vậy phần đường dẫn không cần sửa. Chỉ nên bổ sung: khi rơi vào nhánh dự phòng, UI phải nói rõ
"không tìm thấy công cụ cạnh app — hãy cài bằng file installer, hoặc thêm vào PATH", thay vì
đưa ra một lệnh trông có vẻ chạy được.

**Phần `thydynh03` — đây mới là chỗ hardcode.** Nó đến từ `wails.json:14`:

```json
"companyName": "thydynh03",
```

NSIS lấy giá trị đó làm thư mục cài mặc định (`project.nsi:79`:
`InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"`). Kết quả là **tên
tài khoản GitHub của tác giả nằm trong Program Files của mọi người tải app về**, và nó cũng
là khoá đăng ký gỡ cài đặt (`UNINST_KEY_NAME`).

Việc phải làm: đổi `companyName` thành tên sản phẩm/tổ chức đàng hoàng (ví dụ `Claude Suite`),
để đường dẫn thành `C:\Program Files\Claude Suite\`. Lưu ý đây là **thay đổi phá vỡ tương
thích**: người đã cài bản cũ sẽ có hai thư mục và hai mục trong Apps & features. Cần xử lý
gỡ bản cũ trong `.onInit` của installer, hoặc chấp nhận và ghi rõ trong release note. Nên làm
sớm, càng nhiều người cài thì càng khó đổi.

### 9.1c 🐛 Lệnh join hiện tại **không dùng được cho người ở máy khác**

`thydynh03` không cản trở việc mở phiên: người khác cài app lên máy họ thì thư mục cài trên
*máy họ* cũng tên như vậy, và `claimToolCommand()` vẫn giải ra đúng đường dẫn thật. Xấu về mặt
thương hiệu, nhưng không hỏng chức năng.

Cái thật sự hỏng là hai chỗ khác, và cả hai đều lộ ra đúng lúc có người thứ hai ở **máy khác**:

**Một — `localhost` là địa chỉ của chính máy người nhận.** `app.go:1365`:

```go
host := "ws://" + strings.Replace(addr, "[::]", "localhost", 1)
```

Listener bind mọi giao diện mạng (in ra `[::]`), nhưng chuỗi đưa cho người dùng lại là
`ws://localhost:9111`. Đồng đội dán lệnh đó vào máy họ → `localhost` trỏ về **máy của chính
họ**, nơi không có phiên nào → kết nối thất bại. Lệnh chỉ đúng khi agent thứ hai chạy trên
cùng một máy (mở IDE khác), đúng như ghi chú trong code đã nói. Với đúng kịch bản người dùng
đang hỏi thì nó sai.

**Hai — đường dẫn công cụ trong lệnh là đường dẫn của máy chủ phiên, không phải máy người
nhận.** `claimToolCommand()` chạy trên máy đang mở phiên, nên nó nhét vào chuỗi vị trí cài
đặt *ở đây*. Người nhận có thể đã cài sang ổ D, hoặc dùng bản portable. Hiện tại nó hay chạy
được chỉ vì hai bên tình cờ cùng thư mục mặc định — một sự may mắn, không phải thiết kế.

Việc phải làm:

- **Địa chỉ**: liệt kê IP LAN thật của máy (`net.Interfaces`) và cho chọn; mặc định hiện cả
  hai dòng — *"cùng máy này"* dùng `localhost`, *"máy khác trong mạng"* dùng `ws://192.168.x.x:9111`.
  Kèm cảnh báo tường lửa Windows sẽ hỏi cho phép ở lần đầu.
- **Đường dẫn**: bỏ đường dẫn tuyệt đối ra khỏi lệnh chia sẻ, chỉ dùng `claude-suite-claim`
  (bộ cài nên thêm thư mục app vào PATH — sửa `project.nsi`). Muốn giữ đường dẫn tuyệt đối
  thì phải sinh **hai phiên bản lệnh**: một để chạy tại chỗ, một để gửi cho người khác.
- **Kiểm tra được**: thêm `claude-suite-claim --ping --host ... --session ...` để đồng đội thử
  kết nối trước, thay vì phát hiện sai lúc đang nộp claim.
- Ngoài mạng LAN (khác nơi làm việc) thì cần tunnel — ghi rõ trong tài liệu là chưa hỗ trợ,
  đừng để người dùng tưởng dán lệnh là xong.

### 9.2 ➕ Chat nhiều agent + trọng tài

Hiện `ClaimsPage.svelte` chỉ có nộp claim và xem verdict. Cần dựng thêm:

- **Giao diện chat theo phiên**: mỗi phiên là một phòng, tin nhắn có tác giả, provider, dấu
  thời gian; claim hiển thị như thẻ nhúng trong dòng chat kèm kết quả falsifier.
- **Agent máy khác gửi tin nhắn**: mở rộng giao thức WebSocket sẵn có (`ws://host:9111`)
  thêm loại `message` bên cạnh `claim`. `claude-suite-claim` thêm lệnh con `chat` để agent ở
  IDE máy khác nói chuyện trong phòng.
- **Trọng tài**: một agent giữ vai trò arbiter, đọc toàn bộ tranh luận + kết quả falsifier
  rồi ra phán quyết có lý do. Vẫn giữ nguyên tắc: claim không có falsifier là ý kiến, không
  chặn được merge.
- **Máy bên kia chạy ngầm**: `claude-suite-claim --watch` giữ kết nối, nhận task được giao,
  chạy, trả kết quả về phòng.

### 9.3 Workspace không có go.mod / package.json

`app.go:1322` đã có cảnh báo đúng nội dung: không tìm thấy check nào thì mọi claim thành ý
kiến. Nhưng nó chỉ báo, không chỉ đường.

Việc phải làm: kèm nút **"Tạo file checks"** sinh `.claude-suite/checks.json` mẫu theo ngôn
ngữ phát hiện được trong workspace (Python → pytest/ruff, Node → npm test, Rust → cargo
test, Java → mvn test…), và cho phép tự khai báo lệnh bất kỳ. Có vậy người dùng mới dùng
được cho dự án của họ.

---

## 10. Browser Agent

Đã có sẵn checkbox `Screenshot` và `Headless` (`BrowserAgentPage.svelte:281,289`) — phần này
báo thiếu nhưng thực tế đã có. Cái còn thiếu là **quan sát trong lúc chạy**:

- Đẩy từng bước agent làm (điều hướng, click, gõ, chụp màn hình) vào Live Log theo thời gian
  thực, thay vì chỉ có kết quả cuối.
- **Xem màn hình agent đang làm**: chụp liên tục ~2 giây/lần rồi hiển thị như luồng hình
  ảnh. Đây là cách rẻ và chắc chắn hoạt động; cắm CDP screencast cho mượt hơn có thể làm sau.
- Nút Dừng khi đang chạy.

> Lưu ý ràng buộc đã ghi trong `docs/ARCHITECTURE_DECISIONS.md`: browser agent dùng Chrome
> profile riêng. Đừng đổi sang profile mặc định của người dùng để "tiện hơn" — cách đó đã thử
> và làm mất cookie.

---

## 11. Git — xem nhánh trực quan

`GitPanel.svelte:159` mới liệt kê nhánh dạng danh sách chữ. Cần thêm **đồ thị commit**: vẽ
các đường nhánh/merge bằng SVG từ `GetGitLog` (bổ sung trường parent hashes), commit nào
thuộc nhánh nào, đánh dấu vị trí HEAD. Kèm hover xem chi tiết commit và bấm để xem diff.

`GitPanel` là trang **đang dùng `addToast` đúng chuẩn** — dùng nó làm mẫu cho các trang khác.

---

## 12. Docs & Support

`DocsPage.svelte:169` bọc nội dung trong:

```svelte
<div class="... whitespace-pre-wrap font-sans">
```

Nội dung là chuỗi markdown (`### `, `**đậm**`) nhưng **không có bộ render markdown nào** — nên
người dùng nhìn thấy nguyên ký tự `###` và `**`, đúng như báo cáo.

Việc phải làm:
- Render markdown thật (dùng thư viện có sẵn trong frontend, sanitize trước khi `@html`).
- **Bỏ toàn bộ emoji hardcode** trong `title` (`🚀`, `🤖`…) — dùng `material-symbols` như
  phần còn lại của app để đồng nhất.
- Nội dung docs đang mô tả sai vài chỗ (nhắc "Claude 4.5 Sonnet", "Opus 4.8") — rà lại cho
  khớp model thật đang dùng.
- Áp dụng tương tự cho `SupportPage.svelte`.

---

## Thứ tự thực hiện

Xếp theo **tác động chia cho công sức**, không theo thứ tự người dùng liệt kê:

**Đợt 1 — hết cảm giác "app bị đơ"** (rẻ, chạm vào mọi thứ)
1. Chuẩn hoá toast + trạng thái loading cho toàn bộ handler (§0)
2. Sửa thông báo Export Report (§2.1)
3. Phản hồi Execute Plan + badge task bị chặn (§2.2)
4. Checkbox pool + hành động hàng loạt (§1.6)
5. Render markdown trang Docs, bỏ emoji hardcode (§12)
6. `--author` tự lấy tên máy (§9.1) + báo rõ khi không tìm thấy công cụ claim (§9.1b)
6c. **Lệnh join dùng được cho máy khác**: chọn IP LAN thay vì `localhost`, bỏ đường dẫn tuyệt đối của máy chủ phiên (§9.1c) — không có mục này thì tính năng phân xử chỉ chạy trong một máy

**Đợt 2 — dữ liệu phải thật**
6b. Đổi `companyName` khỏi tên tài khoản cá nhân, xử lý gỡ bản cũ (§9.1b)
7. Lấy quota + tier thật, bỏ số bịa (§1.1, §1.2)
8. Warmup gọi mạng thật, báo cáo thành công/thất bại (§1.3)
9. Wizard cấu hình GCP OAuth (§1.5)

**Đợt 3 — chức năng còn thiếu**
10. Form tạo agent đầy đủ (§5)
11. QuickCLI streaming + nút dừng (§6)
12. Danh mục MCP + tự thêm (§7)
13. Sắp xếp lại sidebar (§4)
14. Live log + xem màn hình Browser Agent (§10)
15. Đồ thị nhánh Git (§11)

**Đợt 4 — hạng mục lớn, PR riêng**
16. Code Studio thành IDE (§3)
17. Chat + trọng tài cho Claims (§9.2, §9.3)
18. Văn phòng 3D theo quy trình thật (§8)

---

## Nguyên tắc khi làm

Trích từ các lỗi dự án này đã lặp lại (`docs/ARCHITECTURE_DECISIONS.md` và bộ nhớ bug-class):

- **Không bịa dữ liệu để lấp chỗ trống.** `defaultQuotas` là ví dụ điển hình: nó làm UI trông
  hoàn chỉnh trong khi không có gì phía sau, và che mất việc chưa làm suốt nhiều phiên bản.
- **Mỗi hàm mới ở `app.go` phải chạy `wails generate module`.** Sửa tay bindings đã hỏng
  nhiều lần; CI có job bắt lỗi này.
- **Một khả năng mới phải có cùng tên hàm ở cả `app.go` và `backend/tui/task_actions.go`**,
  nếu không `backend/contract` sẽ fail build.
- **Kiểm chứng bằng đo đạc, không bằng suy luận.** Trước khi nói "đã sửa", phải revert bản
  sửa và xác nhận test đỏ.
