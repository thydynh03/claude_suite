/**
 * Turn a flat list of workspace paths into a directory tree.
 *
 * The file explorer listed every path in the workspace as one flat row — 96
 * entries reading `backend\services\exporter.go` in full, sorted by whatever
 * order the scan produced. Finding anything meant reading the whole column.
 */

export type TreeNode = {
  name: string;
  /** Full path for files; the directory prefix for folders. */
  path: string;
  isDir: boolean;
  children: TreeNode[];
};

/** Paths arrive with Windows separators from the Go side and slashes elsewhere. */
function segments(path: string): string[] {
  return path.split(/[\\/]/).filter((s) => s !== '');
}

export function buildTree(paths: string[]): TreeNode[] {
  const root: TreeNode = { name: '', path: '', isDir: true, children: [] };

  for (const path of paths) {
    const parts = segments(path);
    if (parts.length === 0) continue;

    let node = root;
    parts.forEach((part, i) => {
      const isLast = i === parts.length - 1;
      const childPath = isLast ? path : parts.slice(0, i + 1).join('/');

      // Match on both name and kind: a file and a directory can legitimately
      // share a name at the same level (`build` and `build.go`).
      let child = node.children.find((c) => c.name === part && c.isDir === !isLast);
      if (!child) {
        child = { name: part, path: childPath, isDir: !isLast, children: [] };
        node.children.push(child);
      }
      node = child;
    });
  }

  sortTree(root);
  return root.children;
}

function sortTree(node: TreeNode) {
  node.children.sort((a, b) => {
    // Directories first, then case-insensitive by name — the order every file
    // browser uses, so it needs no explanation to the person reading it.
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
  });
  node.children.forEach(sortTree);
}

/**
 * Fuzzy match in the style of an editor's quick-open: the query characters must
 * appear in order, not necessarily adjacent, so "bsx" finds
 * "backend/services/exporter.go".
 */
export function fuzzyMatch(query: string, target: string): boolean {
  const q = query.toLowerCase();
  const t = target.toLowerCase();
  if (q === '') return true;

  let qi = 0;
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) qi++;
  }
  return qi === q.length;
}

/** Score a match so exact and prefix hits sort above scattered ones. */
export function matchScore(query: string, target: string): number {
  const q = query.toLowerCase();
  const t = target.toLowerCase();
  const name = t.split(/[\\/]/).pop() || t;

  if (name === q) return 0;
  if (name.startsWith(q)) return 1;
  if (name.includes(q)) return 2;
  if (t.includes(q)) return 3;
  return 4;
}
