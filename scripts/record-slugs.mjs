// The recorder of the slug vectors: the heading ids that GitHub, VS Code and GitLab render, observed from the viewers
// themselves, which store's tests require S_GH, S_VS and S_GL to reproduce (spec 10, Detail headings and anchors). It
// writes, under store/testdata/slug, for each viewer a code point file, the id of a heading holding each code point but
// the surrogates, and a heading file, the ids of the structural cases below; for GitHub also the id and name attributes
// it renders for raw HTML. GitHub is asked through `gh api` (POST /markdown, mode markdown, the mode that renders heading
// anchors); VS Code is run: the latest release's slugify.ts and the #tokenToPlainText method of markdownEngine.ts of
// extensions/markdown-language-features, at the release tag, with the markdown-it version its package-lock.json pins,
// configured as the preview configures it by default, and the markdown-it-katex plugin that its markdown-math
// extension contributes by default; GitLab is run: the gitlab-glfm-markdown gem that GitLab's Gemfile.lock pins, built
// from its own sources, with the options of GitLab's engine, and also with the heading slug prefix GitLab gives a
// snippet's blob alone. It needs the network, an authenticated gh, npm and cargo; `make slug-vectors` runs it.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

/** The repository root: the parent of the folder that holds this script. */
export const repositoryRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));

/** The folder the vectors are written to. */
export const vectorDir = path.join(repositoryRoot, 'store', 'testdata', 'slug');

const log = message => console.log(`[record-slugs] ${message}`);

/** The code points a vector covers: every code point but the surrogates, which no UTF-8 text holds. */
export function* codePoints() {
  for (let cp = 0; cp <= 0x10ffff; cp++) {
    if (cp < 0xd800 || cp > 0xdfff) yield cp;
  }
}

const isASCIIPunct = cp => (cp >= 0x21 && cp <= 0x2f) || (cp >= 0x3a && cp <= 0x40) || (cp >= 0x5b && cp <= 0x60) || (cp >= 0x7b && cp <= 0x7e);

/**
 * The two forms in which a heading holds a code point: `ref`, its hexadecimal numeric character reference, which
 * CommonMark renders as the character (U+0000 as U+FFFD), and `raw`, the character itself, an ASCII punctuation
 * character backslash-escaped so that it is literal text. A line feed or a carriage return has no raw form, since it
 * ends the heading's line, and neither has U+0000: GitHub renders no document that holds it (the API returns the text
 * as it was sent), and CommonMark reads it as U+FFFD, which the reference form records.
 */
export function forms(cp) {
  const ref = `&#x${cp.toString(16).toUpperCase()};`;
  const htmlRaw = cp === 0x0a || cp === 0x0d || cp === 0 || cp === 0x3c || cp === 0x26 ? null : String.fromCodePoint(cp);
  if (cp === 0x0a || cp === 0x0d || cp === 0) return { ref, raw: null, htmlRef: ref, htmlRaw };
  const ch = String.fromCodePoint(cp);
  return { ref, raw: isASCIIPunct(cp) ? `\\${ch}` : ch, htmlRef: ref, htmlRaw };
}

/**
 * The line of a heading of text in a form: an ATX heading, or, for the forms htmlRef and htmlRaw, a raw HTML <h2>
 * heading, an HTML block of its own, which holds the code point as its numeric reference or as itself (no '<' or '&',
 * which would begin markup, and no line ending or U+0000).
 */
const headingLine = (form, text) => (form.startsWith('html') ? `<h2>${text}</h2>` : `# ${text}`);

/** The number of code points a group heading holds. */
export const groupSize = 512;

/**
 * The heading lines of the code points cps in one form: groups of groupSize code points, `# g<n>-c1-c2-…-ck-`, whose
 * id splits at its hyphens into the id of each code point; the space and the hyphen, which write a hyphen, are each
 * read alone, in `# s<hex>p<c>q`. Each heading starts with its own marker so that no two ids are equal and none is
 * numbered.
 */
export function headingsOf(cps, form) {
  const headings = [];
  const grouped = [];
  for (const cp of cps) {
    const f = forms(cp)[form];
    if (f === null) continue;
    if (cp === 0x20 || cp === 0x2d) headings.push({ kind: 'single', cps: [cp], line: headingLine(form, `s${cp.toString(16)}p${f}q`) });
    else grouped.push([cp, f]);
  }
  for (let i = 0; i < grouped.length; i += groupSize) {
    const part = grouped.slice(i, i + groupSize);
    headings.push({ kind: 'group', cps: part.map(([cp]) => cp), line: headingLine(form, `g${headings.length}-${part.map(([, f]) => f).join('-')}-`) });
  }
  return headings;
}

/**
 * The id of each code point of a heading read from the heading's id, a map from code point to id: a group's id splits
 * at its hyphens, and a single's is its id between the marker and q. A heading whose id does not split as expected is
 * returned in `again`, to be read one code point per heading.
 */
export function splitIds(heading, id) {
  const out = new Map();
  if (heading.kind === 'single') {
    const head = `s${heading.cps[0].toString(16)}p`;
    if (!id.startsWith(head) || !id.endsWith('q')) throw new Error(`the heading ${JSON.stringify(heading.line)} rendered the id ${JSON.stringify(id)}`);
    out.set(heading.cps[0], id.slice(head.length, -1));
    return { ids: out, again: false };
  }
  const parts = id.split('-');
  const n = heading.cps.length;
  if (parts.length !== n + 2 || parts[n + 1] !== '') return { ids: out, again: true };
  heading.cps.forEach((cp, k) => out.set(cp, parts[k + 1]));
  return { ids: out, again: false };
}

/** The headings that read each code point of a group alone. */
export function singles(heading, form) {
  return heading.cps.map(cp => ({ kind: 'single', cps: [cp], line: headingLine(form, `s${cp.toString(16)}p${forms(cp)[form]}q`) }));
}

const hex = cp => cp.toString(16).toUpperCase().padStart(4, '0');

/**
 * The treatment of the code point cp whose id is t: `drop` when the id is empty, `keep` when it is the character,
 * `+N` or `-N` (hexadecimal) when it is the single character N code points away, else the code points of the id in
 * hexadecimal joined by spaces; `none` when the form does not exist.
 */
export function treatment(cp, t) {
  if (t === null || t === undefined) return 'none';
  if (t === '') return 'drop';
  const chars = [...t].map(c => c.codePointAt(0));
  if (chars.length === 1) {
    const d = chars[0] - cp;
    if (d === 0) return 'keep';
    return d > 0 ? `+${d.toString(16).toUpperCase()}` : `-${(-d).toString(16).toUpperCase()}`;
  }
  return chars.map(hex).join(' ');
}

/**
 * The lines of a code point file: one line `lo..hi ; ref ; raw` for each run of consecutive code points (the
 * surrogates skipped) whose two treatments are the same, ref that of the reference form and raw that of the raw form.
 */
export function compress(ref, raw) {
  const lines = [];
  let run = null;
  const flush = () => {
    if (run) lines.push(`${hex(run.lo)}..${hex(run.hi)} ; ${run.key}`);
  };
  for (const cp of codePoints()) {
    const key = `${treatment(cp, ref.get(cp))} ; ${treatment(cp, raw.get(cp))}`;
    const next = run && (run.hi + 1 === cp || (run.hi === 0xd7ff && cp === 0xe000));
    if (next && run.key === key) {
      run.hi = cp;
      continue;
    }
    flush();
    run = { lo: cp, hi: cp, key };
  }
  flush();
  return lines;
}

/**
 * The structural cases: documents whose headings' ids show how a viewer reads a heading's text and numbers a repeated
 * slug. Each is recorded as a document of its own, so the numbering starts afresh.
 */
export const structuralCases = [
  ['spaces', '# a b  c   d'],
  ['leading and trailing spaces by reference', '# &#32;a b&#32;'],
  ['tabs', '# a\tb\t\tc'],
  ['tabs by reference', '# &#9;a&#9;b&#9;'],
  ['no-break and ideographic spaces', '# a b　c d'],
  ['line feed by reference', '# a&#10;b'],
  ['underscores', '# a_b __c__ _d_ snake_case_'],
  ['hyphens', '# -a--b- - -'],
  ['ASCII punctuation', '# a!"#$%&\'()*+,./:;<=>?@[\\\\]^`{|}~b'],
  ['character references', '# a&amp;b &copy; &#x41; &#65; &nbsp; &ouml;'],
  ['backslash escapes', '# \\*a\\* \\_b\\_ \\# \\\\'],
  ['emphasis', '# *a* **b** _c_ __d__ ***e***'],
  ['unmatched delimiters', '# *a _b c* d_'],
  ['intraword underscore', '# foo_bar_baz'],
  ['strikethrough', '# ~~a~~ ~b~'],
  ['code spans', '# `a  b` `` c`d `` ` e `'],
  ['code span of spaces', '# x`  `y'],
  ['inline link', '# [a b](/u "t") c'],
  ['reference link', '# [a][r] [r] [r][]\n\n[r]: /u'],
  ['undefined reference', '# [a][nope] [b]'],
  ['autolinks', '# <https://x.y/z> <a@b.c>'],
  ['extended autolinks', '# www.example.com https://x.y/z a@b.co'],
  ['image', '# a ![b c](/i.png) d'],
  ['image with emphasis in its description', '# ![*b* `c`](/i.png)'],
  ['image in a link', '# [![b](/i.png) c](/u)'],
  ['inline HTML', '# a <span id="q">b</span> <!-- c --> d'],
  ['HTML entity in inline HTML', '# a<b title="&amp;">c</b>'],
  ['emoji shortcode', '# a :smile: b'],
  ['footnote reference', '# a[^1]\n\n[^1]: n'],
  ['mention and issue reference', '# @octocat #1 GH-1'],
  ['setext heading with a soft break', 'a\nb\n==='],
  ['setext heading with a hard break by spaces', 'a  \nb\n==='],
  ['setext heading with a hard break by backslash', 'a\\\nb\n==='],
  ['setext heading after a definition', '[r]: /u\na [r]\n---'],
  ['ATX closing sequence', '# a # b ##'],
  ['empty heading', '#\n\n#'],
  ['heading of punctuation only', '# !!!\n\n# ???'],
  ['capitals and dotted I', '# ABC İ K Ω'],
  ['final sigma', '# ΑΣ ΣΑ Σ ΟΔΟΣ'],
  ['decomposed and composed', '# café café'],
  ['marks and numbers', '# x́ ² Ⅳ ١ ½'],
  ['symbols', '# © ™ € → \u{1f600}'],
  ['format characters', '# a​b‍c­d﻿e'],
  ['duplicates', '# a\n\n# a\n\n# a'],
  ['duplicates after a numbered heading', '# a-1\n\n# a\n\n# a'],
  ['a numbered heading between duplicates', '# a\n\n# a-1\n\n# a'],
  ['a numbered heading after duplicates', '# a\n\n# a\n\n# a-1'],
  ['duplicates of a numbered heading', '# a-1\n\n# a-1\n\n# a'],
  ['duplicates under different cases', '# A\n\n# a\n\n# A'],
  ['duplicates of empty headings', '#\n\n#\n\n# -1'],
  ['empty slugs after an empty heading', '#\n\n# !\n\n# ?'],
  ['duplicates of empty slugs', '# !\n\n# ?\n\n# a'],
  ['ten duplicates', Array.from({ length: 11 }, () => '# a').join('\n\n')],
  ['a heading equal to a later number', '# a-2\n\n# a\n\n# a\n\n# a'],
  ['duplicates in a block quote and a list', '> # a\n\n- # a\n\n# a'],
  ['detail heading shape', '## items row T1\n\n## items row T1\n\n## Items Row T1'],
  ['math spans', '# items row $T<b>1$\n\n# items row $T&#49;$\n\n# a $b$ c'],
  ['math spans and code', '# a $`b  c`$ d\n\n# a $$b  c$$ d\n\n# a `$b$` c'],
  ['math delimiters that open nothing', '# a $ b $ c\n\n# a $b $ c\n\n# a $b$1 c\n\n# a$b$c\n\n# a $$$b$$$ c'],
  ['math with escapes', '# a \\$b$ c\n\n# a $b\\$c$ d\n\n# a $b\\\\$ c'],
  ['math in emphasis and links', '# *a $b*$ c*\n\n# [a $]$](/u) b\n\n# ![a $b$](/i.png) c'],
  ['math across a line', 'a $b\nc$ d\n===\n\na $$b\nc$$ d\n==='],
  ['math after inline HTML', '# <span class="x">a $b$ c</span>\n\n# <b>a $b$ c</b>\n\n# <span class="x">*a* $b$</span>'],
  // Footnotes: GitHub and GitLab read footnote definitions and references; GitHub renders a reference as its number.
  ['footnote references by label', '# a[^note] b[^Q] c[^x] d\n\n[^note]: n\n\n[^q]:\n\n[r]: /u'],
  ['footnote references with markup', '# a[^b<i>c] d[^e*f*] g[^h i] ![^note]\n\n[^note]: n'],
  ['footnote numbers by first reference', '# a[^b] c[^a] d[^b]\n\n[^a]: x\n\n[^b]: y'],
  ['footnote numbers from an earlier paragraph', 'x[^b]\n\n# a[^a] c[^b]\n\n[^a]: x\n\n[^b]: y'],
  ['footnote numbers from a definition', '[^a]: has [^b]\n\n# h[^a] g[^b]\n\n[^b]: y'],
  ['footnote numbers from an image and a link', '# a ![y[^m]](/i.png) [x[^n]](/u) [^n]\n\n[^n]: x\n\n[^m]: y'],
  ['footnote labels folded', '# a[^N] b[^n] c[^ss] d[^ß] e[^ n]\n\n[^n]: x\n\n[^ß]: y'],
  ['footnote references beside links', '# a[^n] [^n]x [^n](u) [^n][r] [b [^n]] [^n [c]] [[^n]]\n\n[^n]: x\n\n[r]: /v'],
  ['footnote reference that is a link', '# a[^x y] b[^n]\n\n[^x y]: /u\n\n[^n]: y'],
  ['footnote references in code and HTML', '`[^q]`\n\n<div>\n[^q]\n</div>\n\n# a[^n] b[^q]\n\n[^n]: x\n\n[^q]: y'],
  ['footnote definition interrupting a paragraph', 'items row T1\n[^n]: x\n---'],
  ['footnote definition before a setext underline', '[^n]: x\nitems row T1\n---'],
  ['heading in a referenced footnote definition', '# z[^n]\n\n[^n]: x\n\n    # items row T1\n\n# y'],
  ['heading in an unreferenced footnote definition', '[^n]: x\n\n    # items row T1'],
  ['heading on a footnote definition line', '# z[^n]\n\n[^n]: # items row T1'],
  ['setext heading in a footnote definition', '# z[^n]\n\n[^n]: x\n    items row T1\n    ---'],
  ['footnote definition closed by a short indent', '# z[^n]\n\n[^n]: x\n  \n    # h1'],
  ['footnote definitions in containers', '# a[^n] b[^m]\n\n> [^n]: x\n\n- [^m]: y\n\n      # h1'],
  ['footnote definition nested in a referenced one', '# z[^n]\n\n[^n]: x\n    [^m]: # h1'],
  ['footnote definition nested in an unreferenced one', '# z[^m]\n\n[^n]: x\n    [^m]: # h1'],
  ['duplicate footnote definitions', '# z[^n]\n\n[^n]: # h1\n\n[^n]: # h2'],
  ['duplicate unreferenced footnote definitions', '# z\n\n[^n]: # h1\n\n[^n]: # h2'],
  // Wikilinks: GitLab reads [[title|url]] and [[url]] with wikilinks_title_before_pipe.
  ['wikilinks', '# items row [[T1|x]]\n\n# items row [[T2]]\n\n# a [[ b | c ]] d\n\n# a [[b|c|d]] e'],
  ['wikilinks with markup', '# a [[*b*|c]] d\n\n# e [[f\\|g|h]] i\n\n# j [[k&amp;l|m]] n\n\n# o [[p&#92;*|q]] r\n\n# s [[$t$|u]] v\n\n# w [[x`y`|z]] a'],
  ['wikilinks that open nothing', '# a [x [[b|c]]] e\n\n# f ![[g|h]] i\n\n# j [[k]l]] m\n\n# n \\[[o|p]] q\n\n# r [[s|t] u'],
  ['empty wikilinks', '# a [[|c]] e\n\n# f [[g|]] h\n\n# i [[]] j\n\n# k [[ ]] l'],
  ['wikilink across a line', 'a [[b\nc|d]] e\n==='],
  ['wikilinks in emphasis and footnotes', '# a *[[b*|c]]* e\n\n# f [[^n]] g\n\n[^n]: x'],
  // Math blocks: VS Code's markdown-it-katex reads a line that begins with $$ as a math block.
  ['math block', '$$\n# a\n\n# b\n$$\n# c'],
  ['math block to the end', '$$\n# a\n\n# b'],
  ['one-line math blocks', '$$x$$\n# a\n\n$$$$\n# b\n\n$$a$$b$$\n# c'],
  ['math block opened by a line with one inner delimiter', '$$a $$ b\n# a\n$$\n# b'],
  ['math block interrupting a paragraph', 'items row T1\n$$\nx $$\n---\n\na\n$$x$$\n---\n\nb\n$$c$$d$$\n---'],
  ['paragraph after a math block', '$$\nx $$\nitems row T1\n---'],
  ['indented math blocks', '   $$\n# a\n$$\n# b\n\n    $$\n# c\n$$\n# d'],
  ['math block and a fence', '$$\n```\n$$\n# a\n```\n# b\n\n```\n$$\n```\n# c\n$$\n# d'],
  ['math block in a block quote', '> $$\n> # a\n# b\n\n# c\n\n> $$\nx\ny $$\n# d\n\n> $$\n>\nx\n# e'],
  ['math block ending a block quote', '> a\n$$\n# b\n$$\n# c'],
  ['math block in a list item', '- $$\n  # a\n# b\n# c\n$$\n# d\n\n- $$\n  x\ny\n  # e\n# f'],
  ['math block in nested containers', '- > $$\n  > x\n  y\n  # a\n# b\n\n> - $$\n>   x\n> y\n> # c\n# d'],
  ['math block closed inside a line', '$$\n# a\nx$$y\n# b'],
  ['math block after an HTML block', '<div>\n$$\n</div>\n\n# a\n$$\n# b'],
  // Tables: GitHub, GitLab and VS Code read a GFM table, which takes in the lines after its delimiter row.
  ['table then ATX heading', 'a|b\n-|-\nc|d\n# items row T1'],
  ['table row before a setext underline', 'a|b\n-|-\nitems row T1\n---'],
  ['table row before a setext equals underline', 'a|b\n-|-\nitems row T1\n==='],
  ['table without a body before an underline', 'a|b\n-|-\n---'],
  ['table then a blank line then a setext heading', 'a|b\n-|-\n\nitems row T1\n---'],
  ['delimiter row as a setext underline', 'items row T1\n---|---\n\nitems row T2\n-|-'],
  ['one column table', '| a |\n| - |\nitems row T1\n---\n\n|b|\n|-|\n# h'],
  ['one column table without pipes on the delimiter', '| a |\n---\nx'],
  ['table interrupting a paragraph', 'items row T1\n| a | b |\n| - | - |\n| c | d |\n---'],
  ['table header taking the last line of a paragraph', 'x\ny\na|b\n-|-\nz\n---'],
  ['table with a mismatched delimiter row', 'a|b|c\n-|-\nitems row T1\n---'],
  ['table rows without a pipe', 'a|b\n-|-\nitems row T1\nitems row T2\n---'],
  ['table row that begins a block quote', 'a|b\n-|-\n> # h1\n# h2'],
  ['table row that begins a list', 'a|b\n-|-\n- # h1\n# h2'],
  ['table row that begins an ordered list', 'a|b\n-|-\n1. # h1\n# h2'],
  ['table row that begins a fence', 'a|b\n-|-\n```\n# h1\n```\n# h2'],
  ['table row that begins an HTML block', 'a|b\n-|-\n<div>\n# h1\n</div>\n# h2'],
  ['table row indented four columns', 'a|b\n-|-\n    # h1\n# h2'],
  ['table row that begins a thematic break', 'a|b\n-|-\n***\n# h1'],
  ['table row that begins a footnote definition', 'a|b\n-|-\n[^n]: x\n# h1\n\nz[^n]'],
  ['table row that begins a math block', 'a|b\n-|-\n$$\n# h1\n$$\n# h2'],
  ['table in a block quote', '> a|b\n> -|-\n> # h1\n# h2\n\n> a|b\n> -|-\nitems row T1\n---'],
  ['table in a list item', '- a|b\n  -|-\n  items row T1\n  ---'],
  ['table with escaped pipes', 'a\\|b\n-\\|-\nitems row T1\n---'],
  ['table with a code span pipe', '`a|b`\n-|-\nitems row T1\n---'],
  ['table headed by a heading text', '# a|b\n-|-\nitems row T1\n---'],
  ['table delimiter with alignment', ':-|-:\n:-:|--\nitems row T1\n---\n\na|b\n:-:|-:\nitems row T2\n==='],
  ['table delimiter with spaces', 'a | b\n - | - \nitems row T1\n---'],
  ['table header with a leading pipe only', '|a\n|-\nitems row T1\n---'],
  ['table after a link reference definition', '[x]: /u\na|b\n-|-\nitems row T1\n---'],
];

/**
 * The file a GitLab snippet case is rendered as, and GitLab's heading slug prefix of it: GitLab renders a snippet's
 * markdown blob with use_filename_in_anchor, and Gitlab::HeadingSlug.prefix_from_file_path makes the prefix from the
 * file's base name without its extension, anchorized as comrak anchorizes, and a hyphen.
 */
export const snippetFile = 'items.flowmark.md';

/** Gitlab::HeadingSlug.prefix_from_file_path of file: the anchorized base name without its extension, and '-'. */
export function snippetPrefix(file) {
  const base = path.basename(file, path.extname(file));
  const slug = base.toLowerCase().replace(/[^\p{L}\p{M}\p{N}\p{Pc} -]/gu, '').replaceAll(' ', '-');
  return slug === '' ? '' : `${slug}-`;
}

/** The cases of a GitLab snippet: documents rendered with the heading slug prefix of snippetFile. */
export const snippetCases = [
  ['detail heading shape in a snippet', '## items row T1\n\n## items row T1\n\n# a\n\n# a-1\n\n# a'],
  ['snippet prefix in a heading', `# ${snippetPrefix(snippetFile).slice(0, -1)} items row T1\n\n## items row T1`],
];

/**
 * The raw HTML heading cases: documents with raw HTML <h1>–<h6> elements, beside markdown headings, recorded per heading
 * element: the id each viewer gives it from its text; for a raw element in VS Code and GitLab, which give it none, the
 * id attribute as written, or null.
 */
export const rawHeadingCases = [
  ['raw heading', '<h2>items row T1</h2>'],
  ['raw heading with an id', '<h2 id="hh">g</h2>'],
  ['raw heading in capitals', '<H2>Items Row T1</H2>'],
  ['raw headings of each level', '<h1>a</h1>\n\n<h3>b</h3>\n\n<h6>c</h6>\n\n<h7>d</h7>'],
  ['raw heading with inline markup', '<h3><em>a</em> b</h3>'],
  ['raw heading inline in a paragraph', 'x <h2>a *b* c</h2> d'],
  ['raw heading across a soft break', 'x <h2>a\ny</h2> d'],
  ['raw heading numbered with markdown headings', '# g\n\n<h2>g</h2>\n\n# g'],
  ['raw and markdown heading of a detail shape', '## items row T1\n\n<h2>items row T1</h2>'],
  ['raw heading with character references', '<h2>a&amp;b &#84;1 &copy &AMP; &Tcedil; &apos; &#x83; &#0;</h2>'],
  ['raw heading in a block quote', '> <h2>a</h2>'],
  ['raw heading in a list item', '- <h2>a</h2>'],
  ['raw heading in a div', '<div><h2>a</h2></div>'],
  ['raw heading in pre', '<pre>\n<h2>a</h2>\n</pre>'],
  ['raw heading in code is no heading', '    <h2>a</h2>\n\n`<h2>b</h2>`'],
  ['raw heading with attributes', '<h2 class="x" align="center">a</h2>'],
  ['raw heading with a line break', '<h2>a<br>b</h2>\n\n<h2>c\nd</h2>'],
  ['raw heading with an image', '<h2>a <img alt="b" src="/i.png"> c</h2>'],
  ['raw heading with markdown syntax', '<h2>`a` *b* $c$ _d_</h2>'],
  ['raw heading with a comment, a CDATA section and a processing instruction', '<h2>a<!-- c -->b<![CDATA[x]]>c<?y z?>d</h2>'],
  ['raw heading with filtered tags', '<h2>a<script>x</script>b<title>y</title><style>z</style></h2>'],
  ['raw heading with more filtered tags', '<h2>a <xmp>b</xmp> <textarea>c</textarea> <iframe>d</iframe> <noembed>e</noembed> <noframes>f</noframes> <plaintext>g</h2>'],
  ['markdown heading with filtered tags', '# items row T<script>x</script> y'],
  ['markdown heading cut by a paragraph tag', '# a <p>b</p> c'],
  ['raw heading cut by a paragraph tag', '<h2>a<p>b</p>c</h2>'],
  ['raw heading cut by a table tag', '<h2>a<table><tr><td>b</td></tr></table></h2>'],
  ['raw heading cut by a form', '<h2>a<form>b</form>c</h2>'],
  ['raw heading cut by another heading', '<h2>a<h3>b</h3>c</h2>\n\n<h2>d<h2>e</h2>'],
  ['raw heading kept across block elements', '<h2>a<div>b</div>c<ul><li>d</li></ul>e<blockquote>f</blockquote>g<hr>h</h2>'],
  ['raw heading with an end tag of another heading', '<h2>a</h3>b'],
  ['raw heading closed twice', '<h2>a</h2>b</h2>'],
  ['self-closed raw heading', '<h2/>x'],
  ['unclosed raw heading at the end', '<h2>a'],
  ['raw heading of spaces', '<h2>  a  b  </h2>'],
  ['raw heading of nothing but a line ending', '<h2>\n\n*a* b\n\n</h2>'],
  ['raw heading across a blank line', '<h2>a\n\nb</h2>'],
  ['raw heading continued by an HTML block', '<h2>items row \n\n<div>T1</div>\n\n</h2>'],
  ['raw heading continued by a paragraph in a div', '<h2>items row <div>\n\nT1\n\n</div></h2>'],
  ['raw heading continued by a list', '<h2>items row\n\n- T1\n\n</h2>'],
  ['raw heading continued by a block quote', '<h2>items row\n\n> T1\n\n</h2>'],
  ['raw heading continued by a fenced code block', '<h2>items row\n\n```\nT1\n```\n\n</h2>'],
  ['raw heading continued by an indented code block', '<h2>items row\n\n    T1\n\n</h2>'],
  ['raw heading continued by a thematic break', '<h2>items row\n\n---\n\nT1</h2>'],
  ['raw heading cut by a markdown heading', '<h2>items row\n\n# T1\n\n</h2>'],
  ['raw heading cut by a table', '<h2>items row\n\n| a |\n|---|\n| T1 |\n\n</h2>'],
  ['raw heading cut by a paragraph', '<h2>items row\n\n<span>T1</span>\n\n</h2>'],
  ['raw heading cut by a paragraph after a comment', '<h2>items row\n<!-- x -->\n\nT1'],
  ['raw heading in a paragraph cut by the next paragraph', 'a <h2>items row\n\n<b>T1</b>'],
  ['raw heading with an open element continued', '<h2><span>items row\n\nT1'],
  ['raw heading in a markdown heading', '# x <h2>a</h2> y'],
  ['raw heading with an end tag of an outer element', '<div><h2>a</div>b</h2>'],
  ['raw heading ended by a body end tag', '<h2>a</body>b'],
  ['raw headings of other text', '<h2>Notes</h2>\n\n<h2>items row T1<b>x</b></h2>'],
  ['raw heading whose end tag a div inside it outranks', '<h2>a<div>b</h2>c</div>d'],
  ['raw heading with two elements open at a closer', '<h2><b><i>a<p>b</h2>\n\n<h2><b><span>c<p>d</h2>'],
  ['raw heading with an element closed by another start tag', '<h2><p>a<div>b<p>c</h2>'],
  ['raw heading nested in an unclosed element of an earlier block', '<div>\n\n<h2>a</div>b</h2>'],
  ['raw heading with an isindex', '<h2>a<isindex>b</h2>'],
];

/** The element names whose start tag and end tag GitHub's closers vector records inside a raw <h2>. */
export const elementNames = `a abbr acronym address applet area article aside audio b base basefont bdi bdo big blink
blockquote body br button canvas caption center cite code col colgroup data datalist dd del details dfn dialog dir div
dl dt em embed fieldset figcaption figure font footer form frame frameset h1 h2 h3 h4 h5 h6 head header hgroup hr html
i iframe img input ins isindex kbd keygen label legend li link listing main map mark marquee menu menuitem meta meter
nav nobr noembed noframes noscript object ol optgroup option output p param picture plaintext pre progress q rb rp rt
rtc ruby s samp script search section select slot small source span strike strong style sub summary sup svg math table
tbody td template textarea tfoot th thead time title tr track tt u ul var video wbr xmp foo custom-el`.split(/\s+/);

/** The ids and name attributes of the heading elements of html, in order: the id attribute of each, or null. */
export function headingIdAttrs(html) {
  return [...html.matchAll(/<h[1-6]((?=[\s>\/])[^>]*)>/gi)].map(m => {
    const id = (m[1] ?? '').match(/\sid="([^"]*)"/i);
    return id ? unescapeAttr(id[1]) : null;
  });
}

/**
 * The named character references that GitHub decodes inside a raw HTML heading, with the text each renders: every name
 * of the entities package the engine reads (internal/htmlentity), in its own raw <h2>.
 */
function githubEntities() {
  const names = Object.keys(JSON.parse(readFileSync(path.join(repositoryRoot, 'internal', 'htmlentity', 'testdata', 'entities-3.0.1.json'), 'utf8')));
  const html = githubRender(names.map((n, i) => `<h2>e${i}x&${n};x</h2>`).join('\n\n') + '\n');
  const inner = [...html.matchAll(/<h2[^>]*>e(\d+)x([^]*?)x<\/h2>/g)];
  if (inner.length !== names.length) throw new Error(`${inner.length} headings rendered for ${names.length} names`);
  const out = {};
  for (const [, i, body] of inner) {
    const n = names[Number(i)];
    if (body !== `&amp;${n};`) out[n] = unescapeAttr(body);
  }
  return out;
}

/**
 * The raw HTML attribute cases of GitHub: documents of one raw tag whose id or name attribute GitHub renders. GitHub
 * renders no anchor of its own for them; the recorded value is the attribute as rendered.
 */
export const attributeCases = [
  ['id', '<a id="items-row-t1"></a>'],
  ['name', '<a name="items-row-t1"></a>'],
  ['id in capitals', '<a id="ITEMS-ROW-T1"></a>'],
  ['name in capitals', '<a name="Items-Row-T1"></a>'],
  ['id with the prefix', '<a id="user-content-items-row-t1"></a>'],
  ['name with the prefix', '<a name="user-content-items-row-t1"></a>'],
  ['id with the prefix in capitals', '<a id="User-Content-items-row-t1"></a>'],
  ['name with the prefix in capitals', '<a name="USER-CONTENT-items-row-t1"></a>'],
  ['id with the prefix twice', '<a id="user-content-user-content-x"></a>'],
  ['id with a character reference', '<a id="items-row-&#x54;1"></a>'],
  ['unquoted id', '<span id=items-row-t1>x</span>'],
  ['single-quoted name', "<a name='items-row-t1'>x</a>"],
  ['id of an HTML block', '<div id="items-row-t1">\nx\n</div>'],
  ['id in a heading', '# x <a id="items-row-t1"></a>'],
  ['id with a long s', '<a id="items-row-ſ1"></a>'],
  ['id with a Kelvin sign', '<a id="items-row-K1"></a>'],
];

/** The value of an HTML attribute with its character references decoded. */
export function unescapeAttr(v) {
  return v.replace(/&(#x[0-9a-fA-F]+|#[0-9]+|amp|lt|gt|quot|apos|#39);/g, (m, e) => {
    if (e[0] === '#') return String.fromCodePoint(e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10));
    return { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" }[e];
  });
}

/**
 * The heading ids of GitHub's rendering html, one per heading element in order. GitHub writes each heading it anchors
 * in a <div class="markdown-heading"> that ends with the anchor <a id="user-content-<id>" class="anchor">; the first
 * heading element in the wrapper is the anchored one, whose text the id is of (a heading parsed inside it can follow it
 * there once the sanitizer has removed the elements between). Its id is without user-content-; the empty string for an
 * anchor whose id is empty, which GitHub writes without the prefix; null for any other heading element.
 */
export function githubIds(html) {
  const out = [];
  let wrapper = false;
  let first = -1;
  for (const m of html.matchAll(/<div class="markdown-heading">|<a id="([^"]*)" class="anchor"|<h[1-6](?=[\s>\/])[^>]*>/g)) {
    if (m[0] === '<div class="markdown-heading">') {
      if (wrapper) throw new Error(`a heading wrapper inside another at ${m.index}`);
      wrapper = true;
      first = -1;
    } else if (m[0].startsWith('<a ')) {
      if (!wrapper || first < 0) throw new Error(`a heading anchor outside a heading wrapper at ${m.index}`);
      const id = unescapeAttr(m[1]);
      if (id !== '' && !id.startsWith('user-content-')) throw new Error(`a heading anchor without user-content-: ${id}`);
      out[first] = id === '' ? '' : id.slice('user-content-'.length);
      wrapper = false;
    } else {
      if (wrapper && first < 0) first = out.length;
      out.push(null);
    }
  }
  if (wrapper) throw new Error('a heading wrapper without its anchor');
  return out;
}

/** The id and name attributes of GitHub's rendering html other than its heading anchors, in order. */
export function githubAttributes(html) {
  return [...html.matchAll(/\s(id|name)="([^"]*)"(?! class="anchor")/g)].map(m => ({ attr: m[1], value: unescapeAttr(m[2]) }));
}

/** The largest document sent to GitHub in one request, in bytes; the API renders at most 400 KB. */
const requestLimit = 390_000;

/**
 * The attempts of one GitHub request whose connection fails before any response. A recording sends thousands of
 * requests, and one connection that times out ended whole recordings (three runs on 2026-10-13, `dial tcp …: connect:
 * operation timed out`); the request is idempotent, so it is sent again. An HTTP response that is an error, and a
 * failure that outlasts the attempts, end the recording.
 */
const connectAttempts = 3;

/** GitHub's rendering of a document, by POST /markdown in mode markdown, through gh. */
export function githubRender(text) {
  for (let attempt = 1; ; attempt++) {
    try {
      return execFileSync('gh', ['api', '-X', 'POST', '/markdown', '--input', '-'], {
        input: JSON.stringify({ text, mode: 'markdown' }),
        maxBuffer: 1 << 30,
        stdio: ['pipe', 'pipe', 'pipe'],
      }).toString('utf8');
    } catch (err) {
      // gh writes a failure without a response as `Post "<url>": <error>`, and an HTTP error as `HTTP <status>: …`.
      const stderr = String(err.stderr ?? '');
      if (attempt >= connectAttempts || !stderr.startsWith('Post "https://api.github.com/markdown": ')) throw err;
      log(`GitHub: attempt ${attempt} got no response, sent again: ${stderr.trim()}`);
    }
  }
}

/**
 * The ids of the headings, as a viewer renders them: render is called with documents of whole headings that fit the
 * request limit, and returns their ids in order.
 */
function renderHeadings(headings, render) {
  const ids = [];
  let batch = [];
  let size = 0;
  const send = () => {
    if (batch.length === 0) return;
    const got = render(batch.map(h => h.line).join('\n\n') + '\n');
    if (got.length !== batch.length) throw new Error(`${batch.length} headings rendered ${got.length} ids`);
    ids.push(...got);
    batch = [];
    size = 0;
  };
  for (const h of headings) {
    const n = Buffer.byteLength(h.line) + 2;
    if (size + n > requestLimit) send();
    batch.push(h);
    size += n;
  }
  send();
  return ids;
}

/** The id of every code point in one form, as render observes it. */
function codePointIds(form, render, label) {
  const headings = headingsOf(codePoints(), form);
  log(`${label}: ${form} form, ${headings.length} headings`);
  const ids = renderHeadings(headings, render);
  const out = new Map();
  const again = [];
  headings.forEach((h, k) => {
    const r = splitIds(h, ids[k]);
    if (r.again) again.push(...singles(h, form));
    for (const [cp, t] of r.ids) out.set(cp, t);
  });
  if (again.length > 0) {
    log(`${label}: ${form} form, ${again.length} code points read alone`);
    const singleIds = renderHeadings(again, render);
    again.forEach((h, k) => {
      for (const [cp, t] of splitIds(h, singleIds[k]).ids) out.set(cp, t);
    });
  }
  return out;
}

/** The text of a code point file: its header lines, then the compressed lines. */
function codePointFile(header, ref, raw, html = false) {
  const head = [
    ...header.map(l => `# ${l}`),
    '#',
    '# Each line is lo..hi ; ref ; raw: the code points lo to hi (the surrogates skipped), and the id of a heading',
    '# holding the code point between two hyphens, without them: ref when the heading holds it as its numeric',
    ...(html ? [
      '# character reference &#xN;, raw when it holds the character itself; the heading is a raw HTML <h2> heading,',
      '# an HTML block of its own. drop: the id holds nothing; keep: the character; +N or -N: the one character N',
      '# (hexadecimal) code points away; else the code points of the id; none: no such heading (a line ending,',
      "# U+0000, '<' or '&', which have no raw form here).",
    ] : [
      '# character reference &#xN;, raw when it holds the character itself (an ASCII punctuation character',
      '# backslash-escaped). drop: the id holds nothing; keep: the character; +N or -N: the one character N',
      '# (hexadecimal) code points away; else the code points of the id; none: no such heading (a line ending,',
      '# or U+0000, whose raw form GitHub renders no document with).',
    ]),
  ];
  return [...head, ...compress(ref, raw)].join('\n') + '\n';
}

/** Today's date in UTC, as YYYY-MM-DD. */
const today = () => new Date().toISOString().slice(0, 10);

function recordGitHub() {
  const header = [
    "GitHub's heading ids per code point (spec 10, Detail headings and anchors), the vectors of S_GH.",
    `Recorded ${today()} from GitHub's renderer: POST https://api.github.com/markdown, mode "markdown", which`,
    'renders each heading anchor as <a id="user-content-<id>" class="anchor">; the id here is without the prefix.',
    'Re-record with make slug-vectors (scripts/record-slugs.mjs).',
  ];
  const render = text => githubIds(githubRender(text));
  const ref = codePointIds('ref', render, 'GitHub');
  const raw = codePointIds('raw', render, 'GitHub');
  writeFileSync(path.join(vectorDir, 'github-codepoints.txt'), codePointFile(header, ref, raw));
  const htmlHeader = [
    "GitHub's ids of raw HTML <h2> headings per code point (spec 10, Detail headings and anchors), the vectors of",
    "S_GH's reading of the text of a raw HTML heading.",
    ...header.slice(1),
  ];
  const htmlRef = codePointIds('htmlRef', render, 'GitHub');
  const htmlRaw = codePointIds('htmlRaw', render, 'GitHub');
  writeFileSync(path.join(vectorDir, 'github-html-codepoints.txt'), codePointFile(htmlHeader, htmlRef, htmlRaw, true));
  const cases = structuralCases.map(([name, markdown]) => ({ name, markdown, ids: render(markdown + '\n') }));
  const rawHeadings = rawHeadingCases.map(([name, markdown]) => ({ name, markdown, ids: render(markdown + '\n') }));
  const attributes = attributeCases.map(([name, markdown]) => ({ name, markdown, values: githubAttributes(githubRender(markdown + '\n')) }));
  log('GitHub: the element names that end a heading element');
  const closers = Object.fromEntries(elementNames.map(x => [x, {
    open: render(`<h2>a<${x}>b</h2>\n`),
    end: render(`<h2>a</${x}>b</h2>\n`),
  }]));
  // The start tags that end a heading element, each read again after every element name left open inside it.
  const ending = elementNames.filter(x => closers[x].open.length > 1 || closers[x].open[0] === 'a');
  log(`GitHub: ${ending.length} ending tags after each of ${elementNames.length} open elements`);
  const nested = Object.fromEntries(elementNames.map(o => [o, Object.fromEntries(ending.map(x => [x, render(`<h2><${o}>a<${x}>b</h2>\n`)]))]));
  const file = {
    viewer: 'GitHub',
    recorded: today(),
    request: 'POST https://api.github.com/markdown, mode "markdown"; ids without user-content-',
    rerecord: 'make slug-vectors (scripts/record-slugs.mjs)',
    cases,
    rawHeadings,
    attributes,
    htmlEntities: githubEntities(),
    closers,
    nested,
  };
  writeFileSync(path.join(vectorDir, 'github-headings.json'), JSON.stringify(file, null, 1) + '\n');
}

/** The text of a file of microsoft/vscode at ref, through gh. */
function vscodeFile(file, ref) {
  return execFileSync('gh', ['api', '-H', 'Accept: application/vnd.github.raw', `repos/microsoft/vscode/contents/${file}?ref=${ref}`], { maxBuffer: 1 << 28 }).toString('utf8');
}

/** The source of the method named head of a class source: from head to its matching closing brace. */
export function methodSource(src, head) {
  const at = src.indexOf(head);
  if (at < 0) throw new Error(`no ${head} in the source`);
  let depth = 0;
  for (let i = src.indexOf('{', at); i < src.length; i++) {
    if (src[i] === '{') depth++;
    if (src[i] === '}' && --depth === 0) return src.slice(at, i + 1);
  }
  throw new Error(`${head} does not close`);
}

/**
 * The options that VS Code's markdown-math extension.ts passes to @vscode/markdown-it-katex: the boolean members of its
 * `const options = {…}`, and macros, which are the user's markdown.math.macros, empty by default. Any other member is
 * an error, so a changed extension shows here.
 */
export function katexOptions(src) {
  const m = src.match(/const options = \{([^}]*\{[^}]*\}[^}]*|[^}]*)\};/);
  if (!m) throw new Error('no options in markdown-math extension.ts');
  const options = {};
  for (const line of m[1].split('\n').map(l => l.trim().replace(/,$/, '')).filter(Boolean)) {
    const b = line.match(/^(\w+): (true|false)$/);
    if (b) options[b[1]] = b[2] === 'true';
    else if (line === 'macros: { ...settingsMacros }') options.macros = {};
    else throw new Error(`an option of markdown-math that the recorder does not read: ${line}`);
  }
  return options;
}

async function recordVSCode() {
  const tag = JSON.parse(execFileSync('gh', ['api', 'repos/microsoft/vscode/releases/latest']).toString('utf8')).tag_name;
  const base = 'extensions/markdown-language-features';
  const slugify = vscodeFile(`${base}/src/slugify.ts`, tag);
  const engine = vscodeFile(`${base}/src/markdownEngine.ts`, tag);
  const lock = JSON.parse(vscodeFile(`${base}/package-lock.json`, tag));
  const mdVersion = lock.packages['node_modules/markdown-it'].version;
  const nodeVersion = vscodeFile('.nvmrc', tag).trim();
  const ts = JSON.parse(vscodeFile('package-lock.json', tag)).packages['node_modules/typescript'];
  // The markdown-math extension, which VS Code ships with markdown.math.enabled true by default, contributes
  // @vscode/markdown-it-katex to the engine; its options are those of its extension.ts.
  const mathBase = 'extensions/markdown-math';
  const mathExtension = vscodeFile(`${mathBase}/src/extension.ts`, tag);
  const mathLock = JSON.parse(vscodeFile(`${mathBase}/package-lock.json`, tag)).packages;
  const katexPluginVersion = mathLock['node_modules/@vscode/markdown-it-katex'].version;
  const katexVersion = mathLock['node_modules/katex'].version;
  const mathOptions = katexOptions(mathExtension);
  const mathEnabled = JSON.parse(vscodeFile(`${mathBase}/package.json`, tag)).contributes.configuration
    .flatMap(c => Object.entries(c.properties ?? {}))
    .find(([k]) => k === 'markdown.math.enabled')[1].default;
  if (mathEnabled !== true) throw new Error(`markdown.math.enabled defaults to ${mathEnabled}`);
  const work = mkdtempSync(path.join(tmpdir(), 'record-slugs-'));
  try {
    execFileSync('npm', ['install', '--prefix', work, '--no-save', '--ignore-scripts', '--no-audit', '--no-fund', `markdown-it@${mdVersion}`, `typescript@npm:${ts.name ?? 'typescript'}@${ts.version}`, `@vscode/markdown-it-katex@${katexPluginVersion}`, `katex@${katexVersion}`], { stdio: 'ignore' });
    const typescript = (await import(pathToFileURL(path.join(work, 'node_modules', 'typescript', 'lib', 'typescript.js')).href)).default;
    // The TypeScript sources compiled as VS Code compiles them, by the TypeScript its package-lock.json pins.
    const compile = src => typescript.transpileModule(src, { compilerOptions: { target: typescript.ScriptTarget.ES2022, module: typescript.ModuleKind.ESNext } }).outputText;
    writeFileSync(path.join(work, 'slugify.mjs'), compile(slugify));
    const method = methodSource(engine, '#tokenToPlainText(token: MarkdownIt.Token): string {');
    writeFileSync(path.join(work, 'plain.mjs'), compile(`export class Engine {\n${method}\nplain(token: any): string { return this.#tokenToPlainText(token); }\n}\n`));
    const { githubSlugifier } = await import(pathToFileURL(path.join(work, 'slugify.mjs')).href);
    const { Engine } = await import(pathToFileURL(path.join(work, 'plain.mjs')).href);
    const markdownit = (await import(pathToFileURL(path.join(work, 'node_modules', 'markdown-it', 'index.mjs')).href)).default;
    const katexPlugin = (await import(pathToFileURL(path.join(work, 'node_modules', '@vscode', 'markdown-it-katex', 'dist', 'index.js')).href)).default.default;
    let md = markdownit({ html: true, breaks: false, linkify: true, typographer: false });
    md.linkify.set({ fuzzyLink: false });
    md = md.use(katexPlugin, mathOptions);
    const plain = new Engine();
    // The heading ids of #addNamedHeaders: the slug builder of the render, fed the plain text of each heading's
    // inline token.
    const render = text => {
      const builder = githubSlugifier.createBuilder();
      const tokens = md.parse(text, {});
      const ids = [];
      tokens.forEach((t, i) => {
        if (t.type === 'heading_open') ids.push(builder.add(plain.plain(tokens[i + 1])).value);
      });
      return ids;
    };
    // The id attribute of every heading element of the preview's HTML: #addNamedHeaders sets it on each heading_open
    // token; raw HTML is written as it stands.
    const renderHtml = text => {
      const builder = githubSlugifier.createBuilder();
      const tokens = md.parse(text, {});
      tokens.forEach((t, i) => {
        if (t.type === 'heading_open') t.attrSet('id', builder.add(plain.plain(tokens[i + 1])).value);
      });
      return headingIdAttrs(md.renderer.render(tokens, md.options, {}));
    };
    const header = [
      "VS Code's heading ids per code point (spec 10, Detail headings and anchors), the vectors of S_VS.",
      `Recorded ${today()} by running VS Code ${tag}'s own code: ${base}/src/slugify.ts (githubSlugifier) and`,
      `the #tokenToPlainText method of src/markdownEngine.ts, with markdown-it ${mdVersion} as its package-lock.json`,
      `pins, configured as the preview's defaults (html, linkify without fuzzy links, no breaks, no typographer),`,
      `with @vscode/markdown-it-katex ${katexPluginVersion} (katex ${katexVersion}) as ${mathBase} contributes it, enabled by default;`,
      `node ${process.versions.node} (Unicode ${process.versions.unicode}), VS Code ${tag} names node ${nodeVersion}.`,
      'Re-record with make slug-vectors (scripts/record-slugs.mjs).',
    ];
    const ref = codePointIds('ref', render, 'VS Code');
    const raw = codePointIds('raw', render, 'VS Code');
    writeFileSync(path.join(vectorDir, 'vscode-codepoints.txt'), codePointFile(header, ref, raw));
    const file = {
      viewer: 'VS Code',
      recorded: today(),
      request: `VS Code ${tag}: ${base}/src/slugify.ts and markdownEngine.ts #tokenToPlainText, markdown-it ${mdVersion}, @vscode/markdown-it-katex ${katexPluginVersion} with ${JSON.stringify(mathOptions)}, node ${process.versions.node}`,
      rerecord: 'make slug-vectors (scripts/record-slugs.mjs)',
      cases: structuralCases.map(([name, markdown]) => ({ name, markdown, ids: render(markdown + '\n') })),
      rawHeadings: rawHeadingCases.map(([name, markdown]) => ({ name, markdown, ids: renderHtml(markdown + '\n') })),
    };
    writeFileSync(path.join(vectorDir, 'vscode-headings.json'), JSON.stringify(file, null, 1) + '\n');
    // The slugify.ts that was run, whose expression TestVSRemovalIsVSCodeRegex reads, with VS Code's MIT license.
    writeFileSync(path.join(vectorDir, 'vscode-slugify.ts'), slugify);
    writeFileSync(path.join(vectorDir, 'vscode-LICENSE.txt'), vscodeFile('LICENSE.txt', tag));
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}

/**
 * The command line options of the gitlab-glfm-markdown CLI that the OPTIONS of GitLab's GlfmMarkdown engine
 * (lib/banzai/filter/markdown_engines/glfm_markdown.rb) name: each true member as its flag, header_id_prefix as
 * Banzai::Renderer::USER_CONTENT_ID_PREFIX, only_escape_chars as REFERENCE_CHARS. A false member is the CLI's default.
 * Any other value is an error, so a changed engine shows here.
 */
export function glfmArgs(src) {
  const body = src.match(/OPTIONS = \{([^}]*)\}\.freeze/);
  if (!body) throw new Error('no OPTIONS in glfm_markdown.rb');
  const chars = src.match(/REFERENCE_CHARS = %w\[([^\]]*)\]\.freeze/);
  const args = [];
  for (const line of body[1].split('\n').map(l => l.trim().replace(/,$/, '')).filter(Boolean)) {
    const m = line.match(/^(\w+): (.+)$/);
    if (!m) throw new Error(`an OPTIONS line the recorder does not read: ${line}`);
    const flag = `--${m[1].replaceAll('_', '-')}`;
    if (m[2] === 'true') args.push(flag);
    else if (m[2] === 'false') continue;
    else if (m[1] === 'header_id_prefix' && m[2] === 'Banzai::Renderer::USER_CONTENT_ID_PREFIX') args.push(flag, 'user-content-');
    else if (m[1] === 'only_escape_chars' && m[2] === 'REFERENCE_CHARS' && chars) args.push(flag, chars[1].split(/\s+/).join(''));
    else throw new Error(`an OPTIONS value the recorder does not read: ${line}`);
  }
  return args;
}

/**
 * The heading ids of the gitlab-glfm-markdown rendering html, one per heading element in order: the id of a heading
 * comrak renders, which carries data-sourcepos, without user-content-; for raw HTML, which comrak writes as it stands
 * and gives no id of its own, the id attribute as written, or null.
 */
export function gitlabIds(html) {
  return [...html.matchAll(/<h[1-6]((?=[\s>\/])[^>]*)>/gi)].map(m => {
    const attrs = m[1] ?? '';
    if (!/\sdata-sourcepos="/.test(attrs)) {
      const written = attrs.match(/\sid="([^"]*)"/i);
      return written ? unescapeAttr(written[1]) : null;
    }
    const id = attrs.match(/\sid="([^"]*)"/);
    if (!id) throw new Error(`a heading of comrak without an id: ${m[0]}`);
    const v = unescapeAttr(id[1]);
    if (!v.startsWith('user-content-')) throw new Error(`a heading id without user-content-: ${v}`);
    return v.slice('user-content-'.length);
  });
}

/** The text of a URL, or an error naming it. */
async function fetchText(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url}: ${res.status}`);
  return res.text();
}

/**
 * Builds the CLI of gitlab-glfm-markdown at version, whose gem must have the sha256 GitLab's Gemfile.lock records, in
 * work, and returns its path. The CLI is the gem's own binary target (src/main.rs, with src/glfm.rs and
 * src/formatter.rs, which render through comrak as the Ruby extension does); it is built with the gem's Cargo.lock and
 * dependencies, without the Ruby binding (magnus, rb-sys, serde_magnus), which only src/lib.rs uses.
 */
async function buildGlfm(version, sha256, work) {
  const res = await fetch(`https://rubygems.org/downloads/gitlab-glfm-markdown-${version}.gem`);
  if (!res.ok) throw new Error(`gitlab-glfm-markdown ${version}: ${res.status}`);
  const gem = Buffer.from(await res.arrayBuffer());
  const got = createHash('sha256').update(gem).digest('hex');
  if (got !== sha256) throw new Error(`gitlab-glfm-markdown ${version} has sha256 ${got}; GitLab's Gemfile.lock records ${sha256}`);
  const src = path.join(work, 'gem');
  mkdirSync(src);
  writeFileSync(path.join(work, 'gem.tar'), gem);
  execFileSync('tar', ['-xf', path.join(work, 'gem.tar'), '-C', src]);
  execFileSync('tar', ['-xzf', path.join(src, 'data.tar.gz'), '-C', src]);
  const ext = path.join(src, 'ext', 'gitlab_glfm_markdown');
  const toml = readFileSync(path.join(ext, 'Cargo.toml'), 'utf8');
  const section = name => (toml.match(new RegExp(`^\\[${name}\\]\\n([^]*?)(?=^\\[|(?![^]))`, 'm')) ?? [])[1] ?? '';
  const deps = section('dependencies').split('\n').filter(l => !/^(magnus|rb-sys|serde_magnus)\s*=/.test(l)).join('\n');
  const harness = path.join(work, 'harness');
  mkdirSync(harness);
  writeFileSync(path.join(harness, 'Cargo.toml'), [
    '[package]', 'name = "glfm-cli"', `version = "${version}"`, 'edition = "2021"', 'publish = false', '',
    '[[bin]]', 'name = "glfm"', `path = ${JSON.stringify(path.join(ext, 'src', 'main.rs'))}`, '',
    '[dependencies]', deps.trim(), '',
    '[features]', 'default = ["cli"]', section('features').trim(), '',
  ].join('\n'));
  writeFileSync(path.join(harness, 'Cargo.lock'), readFileSync(path.join(ext, 'Cargo.lock')));
  log(`GitLab: building gitlab-glfm-markdown ${version}'s CLI`);
  execFileSync('cargo', ['build', '--release', '--quiet', '--manifest-path', path.join(harness, 'Cargo.toml')], { stdio: 'inherit' });
  return path.join(harness, 'target', 'release', 'glfm');
}

async function recordGitLab() {
  const project = 'https://gitlab.com/api/v4/projects/gitlab-org%2Fgitlab';
  const commit = JSON.parse(await fetchText(`${project}/repository/commits/master`)).id;
  const raw = file => fetchText(`https://gitlab.com/gitlab-org/gitlab/-/raw/${commit}/${file}`);
  const lock = (await raw('Gemfile.lock')).match(/^ {2}gitlab-glfm-markdown \(([0-9.]+)\) sha256=([0-9a-f]{64})$/m);
  if (!lock) throw new Error("no gitlab-glfm-markdown checksum in GitLab's Gemfile.lock");
  const [, version, sha256] = lock;
  const engineFile = 'lib/banzai/filter/markdown_engines/glfm_markdown.rb';
  const engine = await raw(engineFile);
  const args = glfmArgs(engine);
  // GitLab prefixes heading slugs with the file name only where its render context sets use_filename_in_anchor, which
  // its markup blob viewer sets for a snippet's blob alone; a change of either shows here.
  const viewerFile = 'app/models/blob_viewer/markup.rb';
  const viewer = await raw(viewerFile);
  if (!/^\s*h\[:use_filename_in_anchor\] = true if blob\.container\.is_a\?\(Snippet\)$/m.test(viewer)) throw new Error(`${viewerFile} no longer sets use_filename_in_anchor for a snippet alone`);
  if (!/header_slug_prefix: filename_prefix/.test(engine) || !/return unless context\[:use_filename_in_anchor\]/.test(engine)) throw new Error(`${engineFile} no longer prefixes slugs by use_filename_in_anchor alone`);
  const headingSlug = await raw('lib/gitlab/heading_slug.rb');
  if (!headingSlug.includes('name = File.basename(file_path, File.extname(file_path))') || !headingSlug.includes('"#{slug}-"')) throw new Error('lib/gitlab/heading_slug.rb no longer makes the prefix of the base name');
  const prefix = snippetPrefix(snippetFile);
  const work = mkdtempSync(path.join(tmpdir(), 'record-slugs-'));
  try {
    const bin = await buildGlfm(version, sha256, work);
    const render = text => gitlabIds(execFileSync(bin, args, { input: text, maxBuffer: 1 << 30 }).toString('utf8'));
    const options = args.join(' ');
    const header = [
      "GitLab's heading ids per code point (spec 10, Detail headings and anchors), the vectors of S_GL.",
      `Recorded ${today()} by running gitlab-glfm-markdown ${version}, the version and sha256 GitLab's Gemfile.lock`,
      `pins at commit ${commit}, with the OPTIONS of its ${engineFile}:`,
      options,
      'Each heading id is user-content- and the id here. Re-record with make slug-vectors (scripts/record-slugs.mjs).',
    ];
    const ref = codePointIds('ref', render, 'GitLab');
    const rawIds = codePointIds('raw', render, 'GitLab');
    writeFileSync(path.join(vectorDir, 'gitlab-codepoints.txt'), codePointFile(header, ref, rawIds));
    const file = {
      viewer: 'GitLab',
      recorded: today(),
      request: `gitlab-glfm-markdown ${version} (GitLab ${commit}), ${options}; ids without user-content-`,
      rerecord: 'make slug-vectors (scripts/record-slugs.mjs)',
      cases: structuralCases.map(([name, markdown]) => ({ name, markdown, ids: render(markdown + '\n') })),
      rawHeadings: rawHeadingCases.map(([name, markdown]) => ({ name, markdown, ids: render(markdown + '\n') })),
      snippet: {
        file: snippetFile,
        prefix,
        grounding: `${viewerFile} sets use_filename_in_anchor only for a snippet's blob; ${engineFile} then passes header_slug_prefix`,
        cases: snippetCases.map(([name, markdown]) => ({
          name,
          markdown,
          ids: gitlabIds(execFileSync(bin, [...args, '--header-slug-prefix', prefix], { input: markdown + '\n', maxBuffer: 1 << 30 }).toString('utf8')),
        })),
      },
    };
    writeFileSync(path.join(vectorDir, 'gitlab-headings.json'), JSON.stringify(file, null, 1) + '\n');
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  mkdirSync(vectorDir, { recursive: true });
  const which = process.argv.slice(2);
  if (which.length === 0 || which.includes('github')) recordGitHub();
  if (which.length === 0 || which.includes('vscode')) await recordVSCode();
  if (which.length === 0 || which.includes('gitlab')) await recordGitLab();
  log(`wrote ${vectorDir}`);
}
