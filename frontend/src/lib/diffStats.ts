import { Chunk } from '@codemirror/merge'
import { Text } from '@codemirror/state'

// countDiffChunks reports how many changed regions the merge view will draw
// for two texts — Chunk.build is the exact computation MergeView itself runs,
// so the badge on the Visual Diff button counts what the view then shows.
// (presentableDiff was the first attempt; it returns word-level ranges, so
// one edited line could count as several "changes" the view draws as one.)
//
// It replaces a positional line comparison (line i against line i), under
// which inserting one line near the top counted every following line as
// changed: the badge said hundreds while the real edit was one insertion —
// and, worse, the panes it fed rendered those shifted-but-equal lines with no
// highlight at all.
export function countDiffChunks(original: string, modified: string): number {
  if (original === modified) return 0
  // Same normalisation as DiffView: CodeMirror documents are \n-joined.
  const a = Text.of(original.replace(/\r\n/g, '\n').split('\n'))
  const b = Text.of(modified.replace(/\r\n/g, '\n').split('\n'))
  return Chunk.build(a, b).length
}
