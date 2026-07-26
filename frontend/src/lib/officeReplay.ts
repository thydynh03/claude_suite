// Pure logic behind the 3D office's "Chạy kịch bản" replay — extracted so it
// is testable without WebGL.
//
// The replay used to match tasks to avatars by exact `assigned_to` name only.
// A fresh plan straight from the planner has NO assignments yet (they are set
// at dispatch), and the button itself swapped real avatars for demo personas
// whose names match nothing — so pressing it changed the captions and moved
// nobody. Assignment now falls back the way the real dispatcher does: exact
// name, then role keywords from the task's [TAG], then round-robin, so the
// replay always has actors.

export type ReplayTask = {
  task_id: string;
  title: string;
  assigned_to?: string;
  depends_on?: string[];
};

export function taskTag(title: string): string {
  const m = /^\[([A-Z0-9]+)\]/.exec((title || '').trim());
  return m ? m[1] : '';
}

// Which avatar names plausibly do which kind of work. Mirrors the dispatcher's
// role keywords loosely — this is theatre, not scheduling, so a wrong guess
// costs nothing worse than the wrong actor walking to the meeting room.
const TAG_KEYWORDS: Record<string, string[]> = {
  ARCH: ['architect', 'tech lead', 'arch'],
  BA: ['analyst', '(ba)', 'business'],
  PM: ['manager', 'scrum'],
  CODE: ['develop', 'backend', 'back-end', 'frontend', 'front-end', 'fullstack', 'engineer'],
  QA: ['qa', 'test'],
  E2E: ['e2e', 'tester', 'chrome'],
  DEVOPS: ['devops', 'cloud', 'sre', 'reliability'],
};

// groupTasksIntoLevels orders the board into dependency waves: first the tasks
// nothing blocks, then what those unblock — the order the orchestrator would
// dispatch in. Blockers missing from the board (deleted / other plans) count
// as satisfied, and a dependency cycle ends the walk instead of spinning.
export function groupTasksIntoLevels(tasks: ReplayTask[]): ReplayTask[][] {
  const byId = new Map(tasks.map((t) => [t.task_id, t]));
  const placed = new Set<string>();
  const levels: ReplayTask[][] = [];

  while (placed.size < tasks.length && levels.length < 20) {
    const level = tasks.filter(
      (t) =>
        !placed.has(t.task_id) &&
        (t.depends_on || []).every((d) => !byId.has(d) || placed.has(d))
    );
    if (level.length === 0) break;
    level.forEach((t) => placed.add(t.task_id));
    levels.push(level);
  }
  return levels;
}

// assignLevelToAvatars casts one wave: avatar name → the task it acts out.
// One task per avatar; when everyone is busy the remaining tasks go unacted
// this wave (visible in the caption, which lists every title).
export function assignLevelToAvatars(
  level: ReplayTask[],
  avatarNames: string[],
  rr: { i: number }
): Map<string, ReplayTask> {
  const cast = new Map<string, ReplayTask>();
  if (avatarNames.length === 0) return cast;

  for (const task of level) {
    const name = pickAvatar(task, avatarNames, cast, rr);
    if (name) cast.set(name, task);
  }
  return cast;
}

function pickAvatar(
  task: ReplayTask,
  names: string[],
  taken: Map<string, ReplayTask>,
  rr: { i: number }
): string {
  const preferred: string[] = [];
  if (task.assigned_to) preferred.push(task.assigned_to);
  for (const kw of TAG_KEYWORDS[taskTag(task.title)] || []) {
    for (const n of names) {
      if (n.toLowerCase().includes(kw)) preferred.push(n);
    }
  }
  for (const p of preferred) {
    if (names.includes(p) && !taken.has(p)) return p;
  }
  for (let k = 0; k < names.length; k++) {
    const n = names[(rr.i + k) % names.length];
    if (!taken.has(n)) {
      rr.i = (rr.i + k + 1) % names.length;
      return n;
    }
  }
  return '';
}
