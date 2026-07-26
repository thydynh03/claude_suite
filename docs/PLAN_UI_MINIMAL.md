# Plan 1 — Minimal UI/UX (Claude/Antigravity-class)

Mục tiêu: đưa toàn bộ giao diện về một ngôn ngữ thiết kế tối giản, phẳng, một màu
nhấn, icon nhỏ dạng nét — như Claude desktop / Antigravity. Kết quả phải đồng
nhất trên mọi page, light + dark.

## Nguyên tắc thiết kế (design spec)

1. **Icon**: chỉ dùng Material Symbols (đã self-host, bundle offline). Kích thước
   mặc định 16px (`text-base`), tối đa 20px (`text-xl`) cho khu điều hướng chính.
   KHÔNG emoji trong template làm chrome UI (nút, tab, header, badge, empty state).
   Emoji trong *nội dung log* do backend phát ra không thuộc phạm vi UI.
2. **Màu**: một màu nhấn duy nhất `--primary`. Icon điều hướng dùng màu neutral
   (`on-surface-variant`), KHÔNG tô mỗi item một màu (primary/secondary/tertiary).
   Trạng thái ngữ nghĩa dùng đúng 3 màu dịu: xanh lá (ok), hổ phách (warn), đỏ
   (`--error`) — qua token, không dùng bừa `rose-500`/`emerald-600`/`amber-*`
   trực tiếp khi token có sẵn. Không gradient, không shadow màu.
3. **Chữ**: bỏ ALL-CAPS + tracking rộng cho label điều hướng và nút; dùng
   sentence case, `font-medium` thay `font-bold` đại trà. Thang chữ: 11px phụ,
   12–13px thân, 14px tiêu đề khối, 16px tiêu đề trang.
4. **Khối**: bo góc thống nhất `rounded-lg` (nút, input) và `rounded-xl`
   (card/modal); viền 1px `outline-variant` thay shadow nặng; nền phân tầng bằng
   `surface-container-*`, không hộp-trong-hộp thừa.
5. **Khoảng trắng**: padding thoáng, bỏ badge/pill trang trí không mang thông tin.

## Hạng mục

- [x] **Fonts/icons offline**: bỏ Google Fonts links; bundle `@fontsource/geist-sans`,
  `@fontsource/jetbrains-mono`, `material-symbols` qua npm + main.ts. Sửa mojibake
  `<title>`. (đã làm)
- [ ] **Shell**: TopNavBar + SideNavBar + App.svelte — icon neutral một màu, bỏ
  cờ 🇻🇳/🇺🇸 (thay chữ "VI/EN"), bỏ ALL-CAPS, thu icon toolbar về 16–20px, bỏ
  "v4.8 Orchestrator" hardcode (đọc version thật từ backend), nút Prompt
  Architect bớt nổi.
- [ ] **KanbanView + TaskBoardPage**: cột phẳng, header cột không màu mè, card
  viền 1px, drawer Inspector cùng ngôn ngữ.
- [ ] **CodeStudioPage + GitPanel + DiffView**: thanh công cụ gọn, tab file kiểu
  editor tối giản.
- [ ] **SettingsPage + SchedulerPage + MemoryPage**: form phẳng, section header
  chữ thường, bỏ emoji template.
- [ ] **ClaimsPage + OAuthPoolDashboard + BrowserAgentPage + CockpitPage**: như trên.
- [ ] **Misc**: VirtualOffice3D, AgentRolesPage, Docs, Support, OnboardingTour,
  AgentFormModal, MCPManager, CommandPalette, ToastHost, Dropdown, ModelSelect.

## Chi tiết theo file (từ scan wf_6f420570 — 239 finding UI: 24 high, ~115 medium, ~100 low)

Điểm nặng nhất theo trang (số finding): SettingsPage 24, CodeStudioPage 23,
KanbanView 22, OAuthPoolDashboard 16, CockpitPage 15, TopNavBar 14,
TaskBoardPage 12, ClaimsPage 11, BrowserAgentPage 10.

Các lớp lỗi lặp lại:
- **Emoji làm chrome**: chips hành động, nút update, label toggle, header modal
  (⚠️ hero trong approval modal), cờ 🇻🇳/🇺🇸 trên header.
- **Class không tồn tại**: `border-border` (MemoryPage ~18 chỗ, App.svelte),
  `text-on-surface-muted`, `text-accent`, `font-body-md`,
  `accent-[var(--md-sys-color-primary,#6750a4)]` → viền/màu rơi về currentColor
  hoặc tím Material mặc định.
- **Rainbow palette thô**: rose/emerald/amber/blue/sky/purple + gradient hero
  (TaskBoardPage), panel log bg-black/90 bỏ qua theme sáng.
- **Phím tắt ma**: menu quảng cáo Ctrl+Shift+O/Ctrl++/Ctrl+Shift+P không hề
  bind; item "Command Palette" mở… Settings.
- **UI chết**: quota-fallback modal của TaskBoardPage có state + handler nhưng
  không có markup — flow 429 không bao giờ hiện.
- **Số liệu bịa**: "PLAN CONFIDENCE 98%" là hàm bậc thang của số task.
- **A11y**: outline-none không thay thế, control ẩn opacity-0 nhận focus,
  role=button lồng role=button, hit target ~18px.

Toàn bộ danh sách chi tiết nằm trong scratchpad phiên làm việc
(`findings-ui-*.md`) và được giao cho 6 fixer agent theo nhóm file (workflow
wf_e21390d7): shell / board / studio / settings / ops / misc.

## Điều kiện nghiệm thu

- `git grep` không còn emoji trong template `.svelte` (ngoài phần render log).
- Không còn class màu thô (`rose-*`, `emerald-*`, `amber-*`, hex) ngoài bộ token
  + 3 màu ngữ nghĩa đã định nghĩa tập trung.
- svelte-check 0 lỗi, vitest xanh, npm build xanh; soi bằng mắt các page chính
  ở cả light lẫn dark.
