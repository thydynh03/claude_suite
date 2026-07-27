<!--
  English or Vietnamese, both fine. Tiếng Anh hay tiếng Việt đều được.
  Delete any section that does not apply.
-->

## What this changes / Thay đổi gì

<!-- One or two sentences. The diff shows what; this says why. -->

## Why / Vì sao

<!--
  What went wrong before, or what was impossible. A pull request whose
  description only restates the diff makes the reviewer re-derive the reason.

  Chuyện gì đã sai trước đó, hoặc điều gì trước đây không làm được. Một PR mà
  phần mô tả chỉ kể lại diff sẽ bắt người review tự suy ra lý do.
-->

## How it was verified / Đã kiểm chứng thế nào

<!--
  Say what you actually ran, not what you believe. If you could not verify part
  of it, say which part — that is more useful than silence.

  Ghi lại thứ bạn thật sự đã chạy, không phải thứ bạn tin. Nếu có phần chưa kiểm
  chứng được, hãy nói rõ phần nào — như thế có ích hơn là im lặng.
-->

```
go build ./backend/... ./cmd/...
go vet   ./backend/... ./cmd/...
go test  ./backend/... ./cmd/...
cd frontend && npm run check && npm run test && npm run build
```

## Checklist

- [ ] `go build`, `go vet`, `go test` pass for `./backend/... ./cmd/...`
- [ ] `npm run check`, `npm run test`, `npm run build` pass in `frontend/`
- [ ] `wails generate module` was run if an `App` method or a shared struct changed
- [ ] A new capability uses the **same method name on both frontends** (`backend/contract`)
- [ ] Behaviour changes come with a test that fails without the change
- [ ] User-facing strings exist in both `vi` and `en` (`frontend/src/lib/stores/i18n.ts`)
- [ ] Docs updated in **both languages** if this changes how the app is used
- [ ] No credentials, tokens, or absolute local paths in the diff

## Related / Liên quan

<!-- Closes #123 -->
