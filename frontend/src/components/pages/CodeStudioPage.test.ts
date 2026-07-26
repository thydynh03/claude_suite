import { fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// jsdom cannot host CodeMirror (it measures layout); the stub keeps value and
// the change event observable, which is all these tests assert through. The
// MergeView-based diff pane is stubbed for the same reason — these tests
// never open it, but a stub keeps an accidental instantiation from taking
// the suite down with a layout error.
vi.mock('../ui/CodeEditor.svelte', () => import('../../test/CodeEditorStub.svelte'))
vi.mock('../ui/DiffView.svelte', () => import('../../test/CodeEditorStub.svelte'))

const readFileContent = vi.fn(async (path: string) => `// contents of ${path}\n`)
const saveFileContent = vi.fn(async () => undefined)
const scanWorkspaceFiles = vi.fn(async () => [
  'backend/main.go',
  'backend/services/updater.go',
  'README.md',
])
const runQuickCLI = vi.fn(async () => ({ output: '', error: '' }))

vi.mock('../../../wailsjs/go/main/App', () => ({
  ScanWorkspaceFiles: (...a: unknown[]) => scanWorkspaceFiles(...(a as [])),
  ReadFileContent: (...a: unknown[]) => readFileContent(...(a as [string])),
  SaveFileContent: (...a: unknown[]) => saveFileContent(...(a as [])),
  RunQuickCLI: (...a: unknown[]) => runQuickCLI(...(a as [])),
}))

import CodeStudioPage from './CodeStudioPage.svelte'

beforeEach(() => {
  readFileContent.mockClear()
  readFileContent.mockImplementation(async (path: string) => `// contents of ${path}\n`)
  saveFileContent.mockClear()
  scanWorkspaceFiles.mockClear()
  runQuickCLI.mockClear()
})

describe('CodeStudioPage file tree and tabs', () => {
  it('scans the workspace on mount and opens the first file as a tab', async () => {
    render(CodeStudioPage)

    await waitFor(() => expect(scanWorkspaceFiles).toHaveBeenCalled())
    await waitFor(() => expect(readFileContent).toHaveBeenCalledWith('backend/main.go'))

    // The tab bar names the file (basename), and the tree shows the top level.
    expect((await screen.findAllByText('main.go')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('backend').length).toBeGreaterThan(0)
  })

  it('opens a clicked file in a new tab without re-reading already open ones', async () => {
    render(CodeStudioPage)
    await waitFor(() => expect(readFileContent).toHaveBeenCalledWith('backend/main.go'))

    // README.md sits at the tree root; clicking it must open a second tab.
    const readme = await screen.findByText('README.md')
    await fireEvent.click(readme)

    await waitFor(() => expect(readFileContent).toHaveBeenCalledWith('README.md'))
    const callsBefore = readFileContent.mock.calls.length

    // Clicking it again only activates the existing tab.
    await fireEvent.click(screen.getAllByText('README.md')[0])
    expect(readFileContent.mock.calls.length).toBe(callsBefore)
  })

  it('filters the tree into a flat fuzzy-matched list while searching', async () => {
    render(CodeStudioPage)
    await waitFor(() => expect(scanWorkspaceFiles).toHaveBeenCalled())

    const search = screen.getByPlaceholderText('Tìm tệp tin...')
    await fireEvent.input(search, { target: { value: 'updater' } })

    // The flat result list shows the full path; unrelated files drop out.
    expect(await screen.findByText(/backend[\\/]services[\\/]updater\.go/)).toBeInTheDocument()
    expect(screen.queryByText(/README\.md/)).not.toBeInTheDocument()
  })

  it('shows no-match state for a query nothing fuzzy-matches', async () => {
    render(CodeStudioPage)
    await waitFor(() => expect(scanWorkspaceFiles).toHaveBeenCalled())

    const search = screen.getByPlaceholderText('Tìm tệp tin...')
    await fireEvent.input(search, { target: { value: 'zzzzqqqq' } })

    expect(await screen.findByText('Không tìm thấy file nào')).toBeInTheDocument()
  })
})

describe('CodeStudioPage editing', () => {
  it('enables save on edit, writes the edited content, and is clean afterwards', async () => {
    render(CodeStudioPage)
    await waitFor(() => expect(readFileContent).toHaveBeenCalledWith('backend/main.go'))

    // Nothing edited yet: the save button guards against no-op writes.
    const saveButton = await screen.findByText(/Lưu File \(Ctrl\+S\)/)
    expect(saveButton.closest('button')).toBeDisabled()

    const editor = (await screen.findByTestId('editor-stub')) as HTMLTextAreaElement
    await fireEvent.input(editor, { target: { value: '// edited\n' } })

    await waitFor(() => expect(saveButton.closest('button')).toBeEnabled())

    await fireEvent.click(saveButton.closest('button') as HTMLButtonElement)

    await waitFor(() =>
      expect(saveFileContent).toHaveBeenCalledWith('backend/main.go', '// edited\n'),
    )
    // Saved: the tab is clean again, so the button re-disables.
    await waitFor(() => expect(screen.getByText(/Lưu File \(Ctrl\+S\)/).closest('button')).toBeDisabled())
  })
})

// The AI edit path. The defect these tests pin: the model suggests a few
// spots as a short code block, and the page used to write that block over
// the whole file — a 727-line stylesheet became its own 12-line excerpt.
describe('CodeStudioPage AI edits', () => {
  it('refuses to overwrite the file when the model returns an excerpt', async () => {
    // A file big enough for the half-size guard to mean something.
    const bigFile = Array.from({ length: 100 }, (_, i) => `.rule-${i} { color: red; }`).join('\n')
    readFileContent.mockImplementation(async () => bigFile)
    runQuickCLI.mockResolvedValueOnce({
      output: 'Tôi đề xuất sửa hai chỗ:\n```css\n.rule-1 { color: blue; }\n.rule-2 { color: green; }\n```\n',
      error: '',
    })

    render(CodeStudioPage)
    await waitFor(() => expect(readFileContent).toHaveBeenCalledWith('backend/main.go'))

    await fireEvent.click(await screen.findByText(/Refactor Code/))

    expect(await screen.findByText(/KHÔNG ghi đè/)).toBeInTheDocument()
    expect(saveFileContent).not.toHaveBeenCalled()
  })

  it('applies a full-file result, picking the largest code block over the excerpt', async () => {
    const fullFile = '// refactored contents of backend/main.go\nfunc main() {}\n'
    runQuickCLI.mockResolvedValueOnce({
      // A small before/after excerpt first — the old code took the FIRST
      // block, which is exactly how files got truncated.
      output: 'Chỗ sửa chính:\n```go\n// x\n```\nToàn bộ file sau khi sửa:\n```go\n' + fullFile + '```\n',
      error: '',
    })

    render(CodeStudioPage)
    await waitFor(() => expect(readFileContent).toHaveBeenCalledWith('backend/main.go'))

    await fireEvent.click(await screen.findByText(/Refactor Code/))

    await waitFor(() => expect(saveFileContent).toHaveBeenCalledWith('backend/main.go', fullFile))
  })

  it('never writes the file for an explain turn, even when the answer contains code', async () => {
    runQuickCLI.mockResolvedValueOnce({
      output: 'Luồng xử lý như sau:\n```go\nfunc main() {} // trích dẫn minh hoạ\n```\n',
      error: '',
    })

    render(CodeStudioPage)
    await waitFor(() => expect(readFileContent).toHaveBeenCalledWith('backend/main.go'))

    await fireEvent.click(await screen.findByText(/Giải thích Logic/))

    expect(await screen.findByText(/không bị thay đổi/)).toBeInTheDocument()
    expect(saveFileContent).not.toHaveBeenCalled()
  })
})
