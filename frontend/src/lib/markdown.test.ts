import { describe, it, expect } from 'vitest';
import { renderMarkdown, escapeHtml } from './markdown';

describe('renderMarkdown', () => {
  // The reported defect: docs showed literal ### and ** because the page put raw
  // markdown inside whitespace-pre-wrap.
  it('turns headings and bold into markup instead of showing the syntax', () => {
    const html = renderMarkdown('### Danh sách Agent\n- **Tech Lead**: thiết kế kiến trúc.');

    expect(html).toContain('<h3');
    expect(html).toContain('Danh sách Agent');
    expect(html).toContain('<strong');
    expect(html).toContain('Tech Lead');
    expect(html).not.toContain('###');
    expect(html).not.toContain('**Tech Lead**');
  });

  it('groups consecutive bullets into one list and closes it', () => {
    const html = renderMarkdown('- một\n- hai\n\nĐoạn văn.');

    expect(html.match(/<ul/g)?.length).toBe(1);
    expect(html.match(/<\/ul>/g)?.length).toBe(1);
    expect(html.match(/<li>/g)?.length).toBe(2);
    expect(html).toContain('<p');
  });

  it('leaves markdown syntax alone inside inline code', () => {
    const html = renderMarkdown('Chạy `npm run **build**` nhé');

    expect(html).toContain('<code');
    expect(html).toContain('**build**');
    expect(html).not.toContain('<strong');
  });

  it('renders fenced blocks as preformatted code', () => {
    const html = renderMarkdown('```\ngo build ./...\n```');

    expect(html).toContain('<pre');
    expect(html).toContain('go build ./...');
  });

  // Output is passed to {@html}. Escaping happens before any markdown is applied,
  // so markup in the source cannot reach the DOM as markup.
  it('escapes HTML in the source before rendering', () => {
    const html = renderMarkdown('<img src=x onerror=alert(1)> và **đậm**');

    expect(html).not.toContain('<img');
    expect(html).toContain('&lt;img');
    expect(html).toContain('<strong');
  });

  it('still shows the text when a code fence is never closed', () => {
    const html = renderMarkdown('```\nkhông đóng fence');

    expect(html).toContain('không đóng fence');
  });

  it('returns nothing for empty input', () => {
    expect(renderMarkdown('')).toBe('');
  });
});

describe('escapeHtml', () => {
  it('escapes every character that can start markup', () => {
    expect(escapeHtml(`<>&"'`)).toBe('&lt;&gt;&amp;&quot;&#39;');
  });
});
