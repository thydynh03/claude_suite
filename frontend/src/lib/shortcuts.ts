/**
 * Every keyboard shortcut the app actually binds.
 *
 * One list, because the last one was three lists: the File menu advertised
 * Ctrl+Shift+O and Ctrl+Shift+P, neither of which existed anywhere, and the
 * View menu labelled its reset Ctrl+0 while Ctrl+0 opened the tenth tab. A
 * shortcut sheet that lies is worse than no sheet — the user tries the key,
 * nothing happens, and now they distrust the rest of it.
 *
 * If you bind a key, add it here. If you unbind one, delete it here. The test
 * beside this file checks the claims that can be checked mechanically.
 */
export type Shortcut = {
  keys: string[];
  /** Where the binding lives, so the next person can find it. */
  boundIn: string;
  vi: string;
  en: string;
};

export type ShortcutGroup = {
  vi: string;
  en: string;
  items: Shortcut[];
};

export const SHORTCUT_GROUPS: ShortcutGroup[] = [
  {
    vi: 'Điều hướng',
    en: 'Navigation',
    items: [
      {
        keys: ['Ctrl', 'K'],
        boundIn: 'components/ui/CommandPalette.svelte',
        vi: 'Mở bảng lệnh — tìm và chạy mọi thứ bằng cách gõ',
        en: 'Command palette — find and run anything by typing',
      },
      {
        keys: ['Ctrl', '1'],
        boundIn: 'App.svelte',
        vi: 'Cockpit',
        en: 'Cockpit',
      },
      {
        keys: ['Ctrl', '2'],
        boundIn: 'App.svelte',
        vi: 'Task Board — bảng Kanban và trình lập kế hoạch AI',
        en: 'Task Board — the Kanban board and the AI planner',
      },
      {
        keys: ['Ctrl', '3'],
        boundIn: 'App.svelte',
        vi: 'Trang thứ ba trên thanh điều hướng',
        en: 'The third page on the navigation rail',
      },
      {
        keys: ['Ctrl', '4'],
        boundIn: 'App.svelte',
        vi: 'Trang thứ tư, và cứ thế tới Ctrl+9',
        en: 'The fourth page, and so on up to Ctrl+9',
      },
      {
        keys: ['Ctrl', '0'],
        boundIn: 'App.svelte',
        vi: 'Trang thứ mười trên thanh điều hướng',
        en: 'The tenth page on the navigation rail',
      },
    ],
  },
  {
    vi: 'Phóng to / thu nhỏ',
    en: 'Zoom',
    items: [
      {
        keys: ['Ctrl', 'lăn chuột'],
        boundIn: 'lib/stores/zoom.ts',
        vi: 'Phóng to hoặc thu nhỏ toàn bộ giao diện',
        en: 'Scale the whole interface',
      },
      {
        keys: ['Ctrl', '+'],
        boundIn: 'lib/stores/zoom.ts',
        vi: 'Phóng to một nấc',
        en: 'One step in',
      },
      {
        keys: ['Ctrl', '-'],
        boundIn: 'lib/stores/zoom.ts',
        vi: 'Thu nhỏ một nấc',
        en: 'One step out',
      },
      {
        keys: ['Ctrl', 'Shift', '0'],
        boundIn: 'lib/stores/zoom.ts',
        vi: 'Về 100% — không phải Ctrl+0, phím đó đã là tab thứ mười',
        en: 'Back to 100% — not Ctrl+0, which is already the tenth tab',
      },
    ],
  },
  {
    vi: 'Trong hộp thoại',
    en: 'In dialogs',
    items: [
      {
        keys: ['Esc'],
        boundIn: 'App.svelte',
        vi: 'Đóng hộp thoại phê duyệt — và điều đó có nghĩa là TỪ CHỐI, vì bỏ ngỏ sẽ treo task mãi mãi',
        en: 'Close the approval dialog — which means REJECT, since leaving it open blocks the task forever',
      },
      {
        keys: ['F1'],
        boundIn: 'components/ui/ShortcutSheet.svelte',
        vi: 'Mở đúng bảng bạn đang đọc',
        en: 'Open the sheet you are reading',
      },
    ],
  },
];

/** Flat list, for tests and for search. */
export function allShortcuts(): Shortcut[] {
  return SHORTCUT_GROUPS.flatMap((g) => g.items);
}
