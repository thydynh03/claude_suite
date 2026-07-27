/**
 * Lay out a git commit list as a lane graph.
 *
 * The history panel rendered a flat list, so a repository with branches and
 * merges looked exactly like one with a single straight line — the one thing a
 * history view is for. `git log --all` now reports each commit's parents, which
 * is what makes a layout possible.
 *
 * The algorithm is the standard one: walk the commits newest first, keeping a
 * set of "open lanes", each waiting for a particular commit hash. A commit takes
 * the leftmost lane waiting for it (or a new lane if none is), then hands its
 * lanes to its parents — the first parent inherits the commit's own lane so a
 * straight-line history stays in one column, and further parents (a merge) open
 * lanes of their own.
 */

export type GraphCommit = {
  hash: string;
  message: string;
  author: string;
  date: string;
  parents?: string[];
  refs?: string[];
};

export type LaidOutCommit = GraphCommit & {
  /** Column this commit is drawn in. */
  lane: number;
  /** Total lanes open while this row is drawn, for sizing the gutter. */
  width: number;
  /** Lines to draw from this row down to the next: from lane → to lane. */
  edges: { from: number; to: number }[];
  isMerge: boolean;
};

export function layoutCommits(commits: GraphCommit[]): LaidOutCommit[] {
  // lanes[i] is the hash lane i is currently waiting for; null means free.
  const lanes: (string | null)[] = [];

  const claimLane = (hash: string): number => {
    const existing = lanes.indexOf(hash);
    if (existing !== -1) return existing;

    const free = lanes.indexOf(null);
    if (free !== -1) {
      lanes[free] = hash;
      return free;
    }
    lanes.push(hash);
    return lanes.length - 1;
  };

  const out: LaidOutCommit[] = [];

  for (const commit of commits) {
    const lane = claimLane(commit.hash);
    const parents = commit.parents || [];

    // Release before assigning parents: a commit's own lane is available to its
    // first parent, which is what keeps linear history in a single column.
    lanes[lane] = null;

    const edges: { from: number; to: number }[] = [];
    parents.forEach((parent, index) => {
      let target: number;
      if (index === 0) {
        // Reuse this commit's lane unless another lane already waits for that
        // parent — otherwise two lanes would race for the same commit and the
        // lines would cross for no reason.
        const already = lanes.indexOf(parent);
        if (already !== -1) {
          target = already;
        } else {
          lanes[lane] = parent;
          target = lane;
        }
      } else {
        target = claimLane(parent);
      }
      edges.push({ from: lane, to: target });
    });

    // Trim trailing free lanes so the gutter shrinks once branches are merged
    // rather than staying as wide as the widest point forever.
    while (lanes.length > 0 && lanes[lanes.length - 1] === null) {
      lanes.pop();
    }

    out.push({
      ...commit,
      lane,
      width: Math.max(lanes.length, lane + 1),
      edges,
      isMerge: parents.length > 1,
    });
  }

  return out;
}

/**
 * Stable colour per lane, so a branch keeps its colour down the page.
 *
 * Theme variables, not a seven-hue rainbow: everything else in the Git panel
 * is drawn from the token palette, and a saturated commit graph was the one
 * loud thing left on an otherwise flat surface. The current lane keeps the
 * accent; the rest are neutrals that still differ enough to follow a line.
 * SVG stroke/fill accept var(), and lanes wrap past the end of the list.
 */
export function laneColor(lane: number): string {
  const COLORS = [
    'var(--primary)',
    'var(--outline)',
    'var(--secondary)',
    'var(--outline-variant)',
    'var(--tertiary)',
  ];
  return COLORS[lane % COLORS.length];
}
