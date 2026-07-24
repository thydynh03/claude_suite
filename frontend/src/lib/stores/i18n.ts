import { writable, derived } from 'svelte/store';

export type Language = 'vi' | 'en';

export const currentLang = writable<Language>('vi');

const translations = {
  vi: {
    cockpit: 'Bàn Điều Khiển (Cockpit)',
    kanban: 'Bảng Kanban Kế Hoạch',
    virtual_office: 'Văn Phòng 3D Virtual Office',
    browser_agent: 'Trình Duyệt Agent (Chrome CDP)',
    scheduler: 'Lịch Trình Tự Động (Scheduler)',
    settings: 'Cấu Hình & Tích Hợp',
    code_studio: 'Code Studio',
    support: 'Hỗ Trợ & Trợ Giúp',
    active_agents: 'Agent Đang Hoạt Động',
    system_health: 'Sức Khỏe Hệ Thống',
    memory_usage: 'Bộ Nhớ Đã Dùng',
    tasks_running: 'Tác Vụ Đang Chạy',
    key_pool_status: 'Trạng Thái Key Pool',
    add_account: 'Thêm Tài Khoản Dự Phòng',
    simulate_workflow: 'Chạy Kịch Bản Giả Lập',
    populate_seats: 'Gắn Agent vào 35 Vị Trí'
  },
  en: {
    cockpit: 'Cockpit Dashboard',
    kanban: 'Kanban Board',
    virtual_office: '3D Virtual Office',
    browser_agent: 'Browser Agent (Chrome CDP)',
    scheduler: 'Automated Scheduler',
    settings: 'Settings & Integrations',
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
