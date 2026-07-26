import { describe, it, expect } from 'vitest';
import { groupTasksIntoLevels, assignLevelToAvatars, taskTag, type ReplayTask } from './officeReplay';

const t = (id: string, title: string, deps: string[] = [], assigned = ''): ReplayTask => ({
  task_id: id,
  title,
  depends_on: deps,
  assigned_to: assigned,
});

describe('groupTasksIntoLevels', () => {
  it('orders tasks into dependency waves like the dispatcher would', () => {
    const tasks = [t('a', '[BA] spec'), t('b', '[CODE] build', ['a']), t('c', '[QA] verify', ['b'])];
    const levels = groupTasksIntoLevels(tasks);
    expect(levels.map((l) => l.map((x) => x.task_id))).toEqual([['a'], ['b'], ['c']]);
  });

  it('treats blockers missing from the board as satisfied', () => {
    const levels = groupTasksIntoLevels([t('b', '[CODE] build', ['deleted-task'])]);
    expect(levels).toHaveLength(1);
    expect(levels[0][0].task_id).toBe('b');
  });

  it('stops on a dependency cycle instead of spinning', () => {
    const levels = groupTasksIntoLevels([t('a', 'A', ['b']), t('b', 'B', ['a'])]);
    expect(levels).toEqual([]);
  });
});

describe('assignLevelToAvatars', () => {
  const avatars = ['Tech Lead & Architect', 'Back-end Developer', 'QA/QC Specialist'];

  it('casts EVERY task of a fresh unassigned plan — the bug that made the button do nothing', () => {
    // Fresh plans have assigned_to = '' until dispatch; the replay must still act them.
    const level = [t('1', '[ARCH] thiết kế'), t('2', '[CODE] viết API'), t('3', '[QA] kiểm thử')];
    const cast = assignLevelToAvatars(level, avatars, { i: 0 });

    expect(cast.size).toBe(3);
    expect(cast.get('Tech Lead & Architect')?.task_id).toBe('1');
    expect(cast.get('Back-end Developer')?.task_id).toBe('2');
    expect(cast.get('QA/QC Specialist')?.task_id).toBe('3');
  });

  it('prefers the real assignment when the avatar exists', () => {
    const level = [t('1', '[CODE] fix bug', [], 'QA/QC Specialist')];
    const cast = assignLevelToAvatars(level, avatars, { i: 0 });
    expect(cast.get('QA/QC Specialist')?.task_id).toBe('1');
  });

  it('falls back to round-robin when nothing matches, never leaving the stage idle', () => {
    const level = [t('1', 'việc không tag gì'), t('2', 'việc khác')];
    const cast = assignLevelToAvatars(level, ['Sếp Tổng (CEO)', 'Lễ Tân (Receptionist)'], { i: 0 });
    expect(cast.size).toBe(2);
  });

  it('gives each avatar at most one task per wave', () => {
    const level = [t('1', '[CODE] a'), t('2', '[CODE] b'), t('3', '[CODE] c'), t('4', '[CODE] d')];
    const cast = assignLevelToAvatars(level, ['Back-end Developer', 'Front-end Developer'], { i: 0 });
    expect(cast.size).toBe(2);
  });

  it('handles an empty stage without crashing', () => {
    expect(assignLevelToAvatars([t('1', '[CODE] x')], [], { i: 0 }).size).toBe(0);
  });
});

describe('taskTag', () => {
  it('reads the planner tag prefix', () => {
    expect(taskTag('[QA] kiểm thử đăng nhập')).toBe('QA');
    expect(taskTag('không có tag')).toBe('');
  });
});
