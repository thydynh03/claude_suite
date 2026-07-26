import { describe, it, expect } from 'vitest';
import { layoutCommits, laneColor } from './gitGraph';

const c = (hash: string, parents: string[] = []) => ({
  hash,
  parents,
  message: hash,
  author: 'test',
  date: 'now',
});

describe('layoutCommits', () => {
  // The whole point: a straight history must not fan out into extra columns.
  it('keeps a linear history in a single lane', () => {
    const laid = layoutCommits([c('c', ['b']), c('b', ['a']), c('a')]);

    expect(laid.map((x) => x.lane)).toEqual([0, 0, 0]);
    expect(laid.every((x) => x.width === 1)).toBe(true);
    expect(laid.some((x) => x.isMerge)).toBe(false);
  });

  it('gives a merge two outgoing edges and opens a second lane', () => {
    // m merges branch b into main a.
    const laid = layoutCommits([
      c('m', ['a', 'b']),
      c('a', ['root']),
      c('b', ['root']),
      c('root'),
    ]);

    const merge = laid[0];
    expect(merge.isMerge).toBe(true);
    expect(merge.edges).toHaveLength(2);
    // The two parents must land in different lanes, or the branch is invisible.
    expect(merge.edges[0].to).not.toBe(merge.edges[1].to);
    expect(Math.max(...laid.map((x) => x.width))).toBeGreaterThanOrEqual(2);
  });

  it('reuses the lane once branches converge on a shared parent', () => {
    const laid = layoutCommits([
      c('m', ['a', 'b']),
      c('a', ['root']),
      c('b', ['root']),
      c('root'),
    ]);

    // root is a single commit and must be drawn once, in one lane.
    const roots = laid.filter((x) => x.hash === 'root');
    expect(roots).toHaveLength(1);
    expect(roots[0].width).toBeLessThanOrEqual(2);
  });

  it('gives the root commit no outgoing edges', () => {
    const laid = layoutCommits([c('a')]);
    expect(laid[0].edges).toEqual([]);
  });

  it('handles an empty history', () => {
    expect(layoutCommits([])).toEqual([]);
  });

  // A parent outside the fetched window (limit N) must not break the layout.
  it('tolerates a parent that is not in the list', () => {
    const laid = layoutCommits([c('b', ['a-not-fetched'])]);
    expect(laid).toHaveLength(1);
    expect(laid[0].edges).toHaveLength(1);
  });
});

describe('laneColor', () => {
  it('is stable per lane and wraps around', () => {
    expect(laneColor(0)).toBe(laneColor(0));
    expect(laneColor(0)).not.toBe(laneColor(1));
    expect(laneColor(0)).toBe(laneColor(7));
  });
});
