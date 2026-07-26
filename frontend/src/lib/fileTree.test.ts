import { describe, it, expect } from 'vitest';
import { buildTree, fuzzyMatch, matchScore } from './fileTree';

describe('buildTree', () => {
  it('nests paths into directories instead of one flat list', () => {
    const tree = buildTree(['backend/services/exporter.go', 'backend/cli/claude.go', 'app.go']);

    // Directories first, then files: backend/, then app.go.
    expect(tree.map((n) => n.name)).toEqual(['backend', 'app.go']);
    expect(tree[0].isDir).toBe(true);
    expect(tree[0].children.map((n) => n.name)).toEqual(['cli', 'services']);
    expect(tree[0].children[1].children[0].path).toBe('backend/services/exporter.go');
  });

  // Paths come from Go on Windows, so both separators turn up in the same list.
  it('treats backslashes and slashes as the same separator', () => {
    // String.raw so the backslashes survive as backslashes: written as a normal
    // literal, '\s' is just 's' and the test asserts nothing.
    const tree = buildTree([String.raw`backend\services\git.go`, 'backend/services/exporter.go']);

    expect(tree).toHaveLength(1);
    expect(tree[0].children[0].children.map((n) => n.name)).toEqual(['exporter.go', 'git.go']);
  });

  it('keeps a file and a directory of the same name apart', () => {
    const tree = buildTree(['build/bin/app.exe', 'build.go']);

    expect(tree.map((n) => `${n.name}:${n.isDir}`)).toEqual(['build:true', 'build.go:false']);
  });

  it('sorts case-insensitively', () => {
    const tree = buildTree(['Zebra.go', 'apple.go', 'Banana.go']);
    expect(tree.map((n) => n.name)).toEqual(['apple.go', 'Banana.go', 'Zebra.go']);
  });

  it('ignores empty input and empty paths', () => {
    expect(buildTree([])).toEqual([]);
    expect(buildTree(['', '   /'])).toHaveLength(1);
  });
});

describe('fuzzyMatch', () => {
  it('matches characters in order without requiring them to be adjacent', () => {
    expect(fuzzyMatch('bsx', 'backend/services/exporter.go')).toBe(true);
    expect(fuzzyMatch('exp', 'exporter.go')).toBe(true);
  });

  it('rejects characters that appear out of order', () => {
    expect(fuzzyMatch('xse', 'backend/services/exporter.go')).toBe(false);
  });

  it('matches everything on an empty query', () => {
    expect(fuzzyMatch('', 'anything')).toBe(true);
  });
});

describe('matchScore', () => {
  it('ranks an exact filename above a prefix, above a scattered hit', () => {
    const exact = matchScore('git.go', 'backend/services/git.go');
    const prefix = matchScore('git', 'backend/services/git.go');
    const path = matchScore('services', 'backend/services/git.go');

    expect(exact).toBeLessThan(prefix);
    expect(prefix).toBeLessThan(path);
  });
});
