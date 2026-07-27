import { writable, derived } from 'svelte/store';

export type Language = 'vi' | 'en';

export const currentLang = writable<Language>('vi');

const translations = {
  // Sentence case, short enough not to truncate in the 240px sidebar. Title
  // Case Every Word plus a parenthesised English gloss made every nav item
  // wider than the rail that holds it.
  vi: {
    cockpit: 'Cockpit',
    kanban: 'Task Board',
    virtual_office: 'Văn phòng 3D',
    browser_agent: 'Browser Agent',
    scheduler: 'Lịch tự động',
    settings: 'Cài đặt',
    code_studio: 'Code Studio',
    support: 'Hỗ trợ',
    active_agents: 'Agent đang chạy',
    system_health: 'Tình trạng hệ thống',
    memory_usage: 'Bộ nhớ đã dùng',
    tasks_running: 'Task đang chạy',
    key_pool_status: 'Trạng thái key pool',
    add_account: 'Thêm tài khoản dự phòng',
    simulate_workflow: 'Chạy kịch bản giả lập',
    populate_seats: 'Gắn agent vào 35 vị trí'
  },
  en: {
    cockpit: 'Cockpit',
    kanban: 'Task Board',
    virtual_office: '3D Office',
    browser_agent: 'Browser Agent',
    scheduler: 'Scheduler',
    settings: 'Settings',
    code_studio: 'Code Studio',
    support: 'Support & Help',
    active_agents: 'Active Corporate Agents',
    system_health: 'System Health Telemetry',
    memory_usage: 'Memory Allocation',
    tasks_running: 'Running Tasks',
    key_pool_status: 'Key Pool Status',
    add_account: 'Add Backup Account',
    simulate_workflow: 'Simulate Corporate Workflow',
    populate_seats: 'Populate All 35 Seats'
  }
};

export const t = derived(currentLang, ($lang) => (key: keyof typeof translations['vi']) => {
  return translations[$lang][key] || translations['en'][key] || key;
});
