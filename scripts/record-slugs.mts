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
export const vectorDir = path.join(repositoryRoot, 'packages', 'go', 'store', 'testdata', 'slug');

const log = (message: string): void => console.log(`[record-slugs] ${message}`);

/** The form of a code point in a heading: its reference or itself, in an ATX heading or a raw HTML <h2>. */
export type Form = 'ref' | 'raw' | 'htmlRef' | 'htmlRaw';

/** A heading of the code point vectors: one code point alone, or a group of them, and its line. */
export interface Heading {
  kind: 'single' | 'group';
  cps: number[];
  line: string;
}

/** A viewer's rendering of a document: the id of each heading element of it, in order, or null for one with none. */
export type Render = (text: string) => (string | null)[];

/** A case of the recordings: its name and its markdown. */
export type Case = [name: string, markdown: string];

/** Whether v is a JSON object, or an object of the tools that the recorder runs. */
function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** The member at names of the value v, which what names; the recording fails when a member is missing. */
function at(v: unknown, what: string, ...names: string[]): unknown {
  let cur = v;
  for (const name of names) {
    if (!(isRecord(cur) || typeof cur === 'function') || !(name in cur)) throw new Error(`${what} has no ${names.join('.')}`);
    const next: unknown = Reflect.get(cur, name);
    cur = next;
  }
  return cur;
}

/** The string v, which what names; the recording fails for any other value. */
function text(v: unknown, what: string): string {
  if (typeof v !== 'string') throw new Error(`${what} is no string: ${String(v)}`);
  return v;
}

/** The code points a vector covers: every code point but the surrogates, which no UTF-8 text holds. */
export function* codePoints(): Generator<number> {
  for (let cp = 0; cp <= 0x10ffff; cp++) {
    if (cp < 0xd800 || cp > 0xdfff) yield cp;
  }
}

const isASCIIPunct = (cp: number): boolean => (cp >= 0x21 && cp <= 0x2f) || (cp >= 0x3a && cp <= 0x40) || (cp >= 0x5b && cp <= 0x60) || (cp >= 0x7b && cp <= 0x7e);

/**
 * The two forms in which a heading holds a code point: `ref`, its hexadecimal numeric character reference, which
 * CommonMark renders as the character (U+0000 as U+FFFD), and `raw`, the character itself, an ASCII punctuation
 * character backslash-escaped so that it is literal text. A line feed or a carriage return has no raw form, since it
 * ends the heading's line, and neither has U+0000: GitHub renders no document that holds it (the API returns the text
 * as it was sent), and CommonMark reads it as U+FFFD, which the reference form records.
 */
export function forms(cp: number): Record<Form, string | null> {
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
const headingLine = (form: Form, text: string): string => (form.startsWith('html') ? `<h2>${text}</h2>` : `# ${text}`);

/** The number of code points a group heading holds. */
export const groupSize = 512;

/**
 * The heading lines of the code points cps in one form: groups of groupSize code points, `# g<n>-c1-c2-…-ck-`, whose
 * id splits at its hyphens into the id of each code point; the space and the hyphen, which write a hyphen, are each
 * read alone, in `# s<hex>p<c>q`. Each heading starts with its own marker so that no two ids are equal and none is
 * numbered.
 */
export function headingsOf(cps: Iterable<number>, form: Form): Heading[] {
  const headings: Heading[] = [];
  const grouped: [number, string][] = [];
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
export function splitIds(heading: Heading, id: string | null | undefined): { ids: Map<number, string>; again: boolean } {
  const out = new Map<number, string>();
  if (id === null || id === undefined) throw new Error(`the heading ${JSON.stringify(heading.line)} rendered no id`);
  if (heading.kind === 'single') {
    const [cp] = heading.cps;
    if (cp === undefined) throw new Error(`the heading ${JSON.stringify(heading.line)} holds no code point`);
    const head = `s${cp.toString(16)}p`;
    if (!id.startsWith(head) || !id.endsWith('q')) throw new Error(`the heading ${JSON.stringify(heading.line)} rendered the id ${JSON.stringify(id)}`);
    out.set(cp, id.slice(head.length, -1));
    return { ids: out, again: false };
  }
  const parts = id.split('-');
  const n = heading.cps.length;
  if (parts.length !== n + 2 || parts[n + 1] !== '') return { ids: out, again: true };
  heading.cps.forEach((cp, k) => out.set(cp, parts[k + 1] ?? ''));
  return { ids: out, again: false };
}

/** The headings that read each code point of a group alone. */
export function singles(heading: Heading, form: Form): Heading[] {
  return heading.cps.map(cp => {
    const f = forms(cp)[form];
    if (f === null) throw new Error(`the code point ${hex(cp)} has no ${form} form`);
    return { kind: 'single', cps: [cp], line: headingLine(form, `s${cp.toString(16)}p${f}q`) };
  });
}

const hex = (cp: number): string => cp.toString(16).toUpperCase().padStart(4, '0');

/**
 * The treatment of the code point cp whose id is t: `drop` when the id is empty, `keep` when it is the character,
 * `+N` or `-N` (hexadecimal) when it is the single character N code points away, else the code points of the id in
 * hexadecimal joined by spaces; `none` when the form does not exist.
 */
export function treatment(cp: number, t: string | null | undefined): string {
  if (t === null || t === undefined) return 'none';
  if (t === '') return 'drop';
  const chars = Array.from(t, c => c.codePointAt(0) ?? 0);
  const [only] = chars;
  if (chars.length === 1 && only !== undefined) {
    const d = only - cp;
    if (d === 0) return 'keep';
    return d > 0 ? `+${d.toString(16).toUpperCase()}` : `-${(-d).toString(16).toUpperCase()}`;
  }
  return chars.map(hex).join(' ');
}

/**
 * The lines of a code point file: one line `lo..hi ; ref ; raw` for each run of consecutive code points (the
 * surrogates skipped) whose two treatments are the same, ref that of the reference form and raw that of the raw form.
 */
export function compress(ref: Map<number, string>, raw: Map<number, string>): string[] {
  const lines: string[] = [];
  let run: { lo: number; hi: number; key: string } | null = null;
  const flush = () => {
    if (run) lines.push(`${hex(run.lo)}..${hex(run.hi)} ; ${run.key}`);
  };
  for (const cp of codePoints()) {
    const key = `${treatment(cp, ref.get(cp))} ; ${treatment(cp, raw.get(cp))}`;
    const next = run && (run.hi + 1 === cp || (run.hi === 0xd7ff && cp === 0xe000));
    if (next && run !== null && run.key === key) {
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
export const structuralCases: Case[] = [
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
  ['table header after a link reference definition that a heading uses', '[x]: /u\na|b\n-|-\n\n# [t][x]\n\n[y]: /v\nc\n\n# [t][y]'],
  // Tables, the further cases of the abandoned T12.1-38 attempt (T12.1-38-1): lists, block quotes, HTML blocks,
  // definitions, footnotes, indents and delimiter-row forms, recorded from the viewers like every case above.
  ['table rows over an underline', 'a | b\n--|--\nitems row T1\n---'],
  ['table rows over an equals line', 'a | b\n--|--\nitems row T1\n==='],
  ['table after paragraph lines', 'x\na | b\n--|--\nitems row T1\n---'],
  ['table header of more cells than its delimiter row', 'a | b\n--|--|--\nitems row T1\n---'],
  ['table header of fewer cells than its delimiter row', 'a|\n-|-\nitems row T1\n---'],
  ['table in a block quote before a lazy line', '> a | b\n> --|--\nitems row T1\n---'],
  ['table in a list item before an unindented line', '- a|b\n  -|-\n  c\nitems row T1\n---'],
  ['table one-column header over an underline', 'a|\n---\n\n| b |\n---'],
  ['table one-column header after a paragraph line', 'x\na|\n---'],
  ['table header that a heading reads first', '# items row T1|x\n-|-\n\n> # items row T2|x\n> -|-\n\n- # items row T3|x\n-|-'],
  ['table header that a fence reads first', '```|x\n-|-\n# a\n```\n# b'],
  ['table header that an HTML block reads first', '<div>|x\n-|-\n# a'],
  ['table header that a math block reads first', '$$|x\n-|-\n# a\n$$\n# b'],
  ['table header that a footnote definition reads first', '[^n]: a|b\n-|-\nitems row T1\n---\n\n# z[^n]'],
  ['table header indented three spaces', '   a|b\n-|-\nitems row T1\n---'],
  ['table header indented four spaces in a paragraph', 'x\n    a|b\n-|-\nitems row T1\n---'],
  ['table header indented by a tab', '\ta|b\n-|-\nitems row T1\n---'],
  ['table header indented four spaces in a block quote', '>     a|b\n> -|-\n> items row T1\n> ---'],
  ['table header on a lazy line of a list item', '- x\na|b\n  -|-\n  items row T1\n  ---'],
  ['table header on a lazy line of a block quote', '> x\na|b\n> -|-\n> items row T1\n> ---'],
  ['table delimiter rows that open a table', 'a|b\n|-|-|\nitems row T1\n---\n\na|b\n:-|-:\nitems row T2\n---\n\na|b\n-- | --\nitems row T3\n---\n\na|b\n   -|-\nitems row T4\n---\n\na|b\n| -|-\nitems row T5\n---\n\na|b\n-|-|\nitems row T6\n---'],
  ['table delimiter row with a vertical tab', 'a|b\n-|-\v\nitems row T1\n---'],
  ['table delimiter rows that open none', 'a|b\n\t-|-\nitems row T1\n---\n\na|b\n- | -\nitems row T2\n---\n\na|b|c\n-||-\nitems row T3\n---\n\na|b\n:|:\nitems row T4\n---\n\na|b\n=|=\nitems row T5\n---'],
  ['table delimiter row less indented than a list item', '- a|b\n -|-\nitems row T1\n---'],
  ['table delimiter row deeper in a list item', '- a|b\n    -|-\n  items row T1\n  ---'],
  ['table delimiter row on a lazy line of a block quote', '> a|b\n-|-\nitems row T1\n---'],
  ['table header cells with escaped pipes', 'a\\|b|c\n-|-\nitems row T1\n---\n\na\\|b\n-|-\nitems row T2\n---\n\na\\\\|b\n-|-\nitems row T3\n---\n\na|b\\|\n-|-\nitems row T4\n---'],
  ['table header cells with a code span and outer pipes', '`a|b`\n-|-\nitems row T1\n---\n\n|a|b|\n-|-\nitems row T2\n---\n\na||b\n-|-|-\nitems row T3\n---'],
  ['table header of a lone pipe', '|\n-\nitems row T1\n---\n\n||\n-\nitems row T2\n---'],
  ['table header after definitions', '[r]: /u\na|b\n-|-\nitems row T1\n---\n\n[s]: /u|b\n-|-\nitems row T2\n---'],
  ['table header after a failed header in its paragraph', 'a|b|c\n-|-\nd|e\n-|-\nitems row T1\n---'],
  ['table rows ended by an HTML block of type 7', 'a|b\n-|-\n<span>\n# h'],
  ['table rows ended by an HTML block of type 6', 'a|b\n-|-\n<div>\n# h'],
  ['table rows ended by a footnote definition', 'a|b\n-|-\n[^n]: x\n    # c\n\n# z[^n]'],
  ['table rows ended by a block quote and a fence', 'a|b\n-|-\n> # c\n\na|b\n-|-\n```\n# d\n```'],
  ['table rows ended by an indented line', 'a | b\n--|--\n    x\nitems row T1\n---'],
  ['table rows ended by list items', 'a|b\n-|-\n-\nitems row T1\n---\n\na|b\n-|-\n1.\nitems row T2\n---\n\na|b\n-|-\n2.\nitems row T3\n---'],
  ['table rows ended by a math block', 'a | b\n--|--\n$$\n# a\n$$\n# b'],
  ['table row of a lone pipe', 'a|b\n-|-\n|\nitems row T1\n---\n\na|b\n-|-\n|  \nitems row T2\n---'],
  ['table row of a no-break space', 'a|b\n-|-\n \nitems row T1\n---'],
  ['table in a footnote definition', '[^n]: x\n\n    a|b\n    -|-\n    items row T1\n    ---\n\n# z[^n]'],
  ['table footnote reference in an excess cell', '| a |\n|---|\n| x | y[^n] |\n\n# h[^m]\n\n[^m]: x\n\n[^n]: y'],
  ['table escaped pipe in the paragraph before a header', 'x[^a\\|b]\nc|d\n-|-\n\n# h[^q]\n\n[^a|b]: y\n\n[^q]: z\n\nx[^a\\|b]\nc\n\n# g[^q]'],
  ['table escaped pipe in a cell', 'c|d\n-|-\nx[^a\\|b]\n\n# h[^q]\n\n[^a|b]: y\n\n[^q]: z'],
  ['table escaped backslash before a pipe in a cell', 'c|d\n-|-\nx[^a\\\\|b]\n\n# h[^q]\n\n[^a\\|b]: y\n\n[^q]: z'],
  ['table cell limit', tableCellLimit()],
  ['id in a table cell', '| a |\n|---|\n| <a id="items-row-t1"></a> |'],
  ['id in an excess table cell', '| a |\n|---|\n| x | <a id="items-row-t1"></a> |'],
  ['id after a pipe in a code span of a table cell', '| a | b |\n|---|---|\n| `x | <a id="items-row-t1"></a> ` |'],
];

/**
 * The document of the table cell limit case: a header of 1001 cells over rows of one cell each, x1 to x600, then an
 * underline. Each viewer stops taking rows once the cells it has autocompleted pass its limit, and the rows from there
 * are a paragraph that the underline makes a heading, whose text names the first row the table did not take.
 */
export function tableCellLimit(): string {
  const n = 1001;
  const rows = Array.from({ length: 600 }, (_, k) => `x${k + 1}`);
  return [`|${'a|'.repeat(n)}`, `|${'-|'.repeat(n)}`, ...rows, '---'].join('\n');
}

/**
 * The file a GitLab snippet case is rendered as, and GitLab's heading slug prefix of it: GitLab renders a snippet's
 * markdown blob with use_filename_in_anchor, and Gitlab::HeadingSlug.prefix_from_file_path makes the prefix from the
 * file's base name without its extension, anchorized as comrak anchorizes, and a hyphen.
 */
export const snippetFile = 'items.flowmark.md';

/** Gitlab::HeadingSlug.prefix_from_file_path of file: the anchorized base name without its extension, and '-'. */
export function snippetPrefix(file: string): string {
  const base = path.basename(file, path.extname(file));
  const slug = base.toLowerCase().replace(/[^\p{L}\p{M}\p{N}\p{Pc} -]/gu, '').replaceAll(' ', '-');
  return slug === '' ? '' : `${slug}-`;
}

/** The cases of a GitLab snippet: documents rendered with the heading slug prefix of snippetFile. */
export const snippetCases: Case[] = [
  ['detail heading shape in a snippet', '## items row T1\n\n## items row T1\n\n# a\n\n# a-1\n\n# a'],
  ['snippet prefix in a heading', `# ${snippetPrefix(snippetFile).slice(0, -1)} items row T1\n\n## items row T1`],
];

/**
 * The raw HTML heading cases: documents with raw HTML <h1>–<h6> elements, beside markdown headings, recorded per heading
 * element: the id each viewer gives it from its text; for a raw element in VS Code and GitLab, which give it none, the
 * id attribute as written, or null.
 */
export const rawHeadingCases: Case[] = [
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
  ['raw headings in table cells', '| <h2>items row T1</h2> | b |\n|---|---|\n| <h3>c</h3> | d |'],
  ['raw heading ended by its table cell', '| <h2>items row | T1 |\n|---|---|\n| x | y |'],
  ['raw heading in an excess table cell', '| a |\n|---|\n| x | <h2>items row T1</h2> |'],
  ['raw heading after a pipe in a code span of a table cell', '| a | b |\n|---|---|\n| `x | <h2>items row T1</h2> ` |'],
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
export function headingIdAttrs(html: string): (string | null)[] {
  return [...html.matchAll(/<h[1-6]((?=[\s>/])[^>]*)>/gi)].map(m => {
    const id = /\sid="([^"]*)"/i.exec(m[1] ?? '');
    return id ? unescapeAttr(id[1] ?? '') : null;
  });
}

// The suppression line of slugify.ts of VS Code and the reason that the suppression check of kit asks of it; the recorded
// file keeps the source byte-identical but for this note.
const SLUGIFY_SUPPRESSION = '// eslint-disable-next-line no-misleading-character-class\n';
const SLUGIFY_REASON = '// eslint-disable-next-line no-misleading-character-class -- vendored VS Code source, kept byte-identical except this note\n';

// withReason is slugify.ts with the reason on its suppression line; a release without that line fails the recording.
function withReason(slugify: string): string {
  if (!slugify.includes(SLUGIFY_SUPPRESSION)) throw new Error('slugify.ts of VS Code holds no eslint-disable-next-line no-misleading-character-class line');
  return slugify.replace(SLUGIFY_SUPPRESSION, SLUGIFY_REASON);
}

/**
 * The named character references that GitHub decodes inside a raw HTML heading, with the text each renders: every name
 * of the entities package the engine reads (internal/htmlentity), in its own raw <h2>.
 */
function githubEntities(): Record<string, string> {
  const entities: unknown = JSON.parse(readFileSync(path.join(repositoryRoot, 'packages', 'go', 'internal', 'htmlentity', 'testdata', 'entities-3.0.1.json'), 'utf8'));
  if (!isRecord(entities)) throw new Error('entities-3.0.1.json is no object');
  const names = Object.keys(entities);
  const html = githubRender(names.map((n, i) => `<h2>e${i}x&${n};x</h2>`).join('\n\n') + '\n');
  const inner = [...html.matchAll(/<h2[^>]*>e(\d+)x([^]*?)x<\/h2>/g)];
  if (inner.length !== names.length) throw new Error(`${inner.length} headings rendered for ${names.length} names`);
  const out: Record<string, string> = {};
  for (const [, i, body] of inner) {
    const n = names[Number(i)];
    if (n === undefined || body === undefined) throw new Error(`the heading e${String(i)} names no entity`);
    if (body !== `&amp;${n};`) out[n] = unescapeAttr(body);
  }
  return out;
}

/**
 * The raw HTML attribute cases of GitHub: documents of one raw tag whose id or name attribute GitHub renders. GitHub
 * renders no anchor of its own for them; the recorded value is the attribute as rendered.
 */
export const attributeCases: Case[] = [
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
export function unescapeAttr(v: string): string {
  const named: Record<string, string> = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
  return v.replace(/&(#x[0-9a-fA-F]+|#[0-9]+|amp|lt|gt|quot|apos|#39);/g, (_m: string, e: string) => {
    if (e.startsWith('#')) return String.fromCodePoint(e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10));
    const c = named[e];
    if (c === undefined) throw new Error(`the reference &${e}; is no named reference of the recorder`);
    return c;
  });
}

/**
 * The heading ids of GitHub's rendering html, one per heading element in order. GitHub writes each heading it anchors
 * in a <div class="markdown-heading"> that ends with the anchor <a id="user-content-<id>" class="anchor">; the first
 * heading element in the wrapper is the anchored one, whose text the id is of (a heading parsed inside it can follow it
 * there once the sanitizer has removed the elements between). Its id is without user-content-; the empty string for an
 * anchor whose id is empty, which GitHub writes without the prefix; null for any other heading element.
 */
export function githubIds(html: string): (string | null)[] {
  const out: (string | null)[] = [];
  let wrapper = false;
  let first = -1;
  for (const m of html.matchAll(/<div class="markdown-heading">|<a id="([^"]*)" class="anchor"|<h[1-6](?=[\s>/])[^>]*>/g)) {
    if (m[0] === '<div class="markdown-heading">') {
      if (wrapper) throw new Error(`a heading wrapper inside another at ${m.index}`);
      wrapper = true;
      first = -1;
    } else if (m[0].startsWith('<a ')) {
      if (!wrapper || first < 0) throw new Error(`a heading anchor outside a heading wrapper at ${m.index}`);
      const id = unescapeAttr(m[1] ?? '');
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
export function githubAttributes(html: string): { attr: string; value: string }[] {
  return [...html.matchAll(/\s(id|name)="([^"]*)"(?! class="anchor")/g)].map(m => ({ attr: m[1] ?? '', value: unescapeAttr(m[2] ?? '') }));
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

/** Sends one POST /markdown request through gh with the JSON body input and returns its output. */
function ghMarkdown(input: string): string {
  return execFileSync('gh', ['api', '-X', 'POST', '/markdown', '--input', '-'], {
    input,
    maxBuffer: 1 << 30,
    stdio: ['pipe', 'pipe', 'pipe'],
  }).toString('utf8');
}

/**
 * GitHub's rendering of a document, by POST /markdown in mode markdown, through gh. request sends one request and
 * throws as execFileSync does, with the stderr of gh; the tests pass their own, so no stub program stands in for gh.
 */
export function githubRender(text: string, request: (input: string) => string = ghMarkdown): string {
  for (let attempt = 1; ; attempt++) {
    try {
      return request(JSON.stringify({ text, mode: 'markdown' }));
    } catch (err) {
      // gh writes a failure without a response as `Post "<url>": <error>`, and an HTTP error as `HTTP <status>: …`.
      const stderr = err instanceof Error && 'stderr' in err ? String(err.stderr) : '';
      if (attempt >= connectAttempts || !stderr.startsWith('Post "https://api.github.com/markdown": ')) throw err;
      log(`GitHub: attempt ${attempt} got no response, sent again: ${stderr.trim()}`);
    }
  }
}

/**
 * The ids of the headings, as a viewer renders them: render is called with documents of whole headings that fit the
 * request limit, and returns their ids in order.
 */
function renderHeadings(headings: Heading[], render: Render): (string | null)[] {
  const ids: (string | null)[] = [];
  let batch: Heading[] = [];
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
function codePointIds(form: Form, render: Render, label: string): Map<number, string> {
  const headings = headingsOf(codePoints(), form);
  log(`${label}: ${form} form, ${headings.length} headings`);
  const ids = renderHeadings(headings, render);
  const out = new Map<number, string>();
  const again: Heading[] = [];
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
function codePointFile(header: string[], ref: Map<number, string>, raw: Map<number, string>, html = false): string {
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
const today = (): string => new Date().toISOString().slice(0, 10);

function recordGitHub(): void {
  const header = [
    "GitHub's heading ids per code point (spec 10, Detail headings and anchors), the vectors of S_GH.",
    `Recorded ${today()} from GitHub's renderer: POST https://api.github.com/markdown, mode "markdown", which`,
    'renders each heading anchor as <a id="user-content-<id>" class="anchor">; the id here is without the prefix.',
    'Re-record with make slug-vectors (scripts/record-slugs.mts).',
  ];
  const render: Render = text => githubIds(githubRender(text));
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
  const closerList = elementNames.map(x => [x, {
    open: render(`<h2>a<${x}>b</h2>\n`),
    end: render(`<h2>a</${x}>b</h2>\n`),
  }] as const);
  const closers = Object.fromEntries(closerList);
  // The start tags that end a heading element, each read again after every element name left open inside it.
  const ending = closerList.filter(([, c]) => c.open.length > 1 || c.open[0] === 'a').map(([x]) => x);
  log(`GitHub: ${ending.length} ending tags after each of ${elementNames.length} open elements`);
  const nested = Object.fromEntries(elementNames.map(o => [o, Object.fromEntries(ending.map(x => [x, render(`<h2><${o}>a<${x}>b</h2>\n`)]))]));
  const file = {
    viewer: 'GitHub',
    recorded: today(),
    request: 'POST https://api.github.com/markdown, mode "markdown"; ids without user-content-',
    rerecord: 'make slug-vectors (scripts/record-slugs.mts)',
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
function vscodeFile(file: string, ref: string): string {
  return execFileSync('gh', ['api', '-H', 'Accept: application/vnd.github.raw', `repos/microsoft/vscode/contents/${file}?ref=${ref}`], { maxBuffer: 1 << 28 }).toString('utf8');
}

/** The source of the method named head of a class source: from head to its matching closing brace. */
export function methodSource(src: string, head: string): string {
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
export function katexOptions(src: string): Record<string, boolean | Record<string, never>> {
  const m = /const options = \{([^}]*\{[^}]*\}[^}]*|[^}]*)\};/.exec(src);
  if (!m) throw new Error('no options in markdown-math extension.ts');
  const options: Record<string, boolean | Record<string, never>> = {};
  for (const line of (m[1] ?? '').split('\n').map(l => l.trim().replace(/,$/, '')).filter(Boolean)) {
    const b = /^(\w+): (true|false)$/.exec(line);
    if (b && b[1] !== undefined) options[b[1]] = b[2] === 'true';
    else if (line === 'macros: { ...settingsMacros }') options.macros = {};
    else throw new Error(`an option of markdown-math that the recorder does not read: ${line}`);
  }
  return options;
}

/** VS Code's TypeScript as the recorder runs it: the transpiler of its sources. */
interface Transpiler {
  transpileModule(src: string, options: { compilerOptions: { target: unknown; module: unknown } }): { outputText: string };
  ScriptTarget: { ES2022: unknown };
  ModuleKind: { ESNext: unknown };
}

/** The slug builder of VS Code's slugify.ts. */
interface Slugifier {
  createBuilder(): { add(text: string): { value: string } };
}

/** A token of markdown-it, as the recorder reads it. */
interface MarkdownToken {
  type: string;
  attrSet(name: string, value: string): void;
}

/** markdown-it, as the recorder configures and runs it. */
interface Markdown {
  parse(src: string, env: object): MarkdownToken[];
  use(plugin: unknown, options: unknown): Markdown;
  linkify: { set(options: { fuzzyLink: boolean }): unknown };
  renderer: { render(tokens: MarkdownToken[], options: unknown, env: object): string };
  options: unknown;
}

const isTranspiler = (v: unknown): v is Transpiler => isRecord(v) && typeof v.transpileModule === 'function';
const isSlugifier = (v: unknown): v is Slugifier => isRecord(v) && typeof v.createBuilder === 'function';
const isEngine = (v: unknown): v is new () => { plain(token: MarkdownToken): string } => typeof v === 'function';
const isMarkdownFactory = (v: unknown): v is (options: { html: boolean; breaks: boolean; linkify: boolean; typographer: boolean }) => Markdown => typeof v === 'function';

/** The module of the file in the work folder, through import(). */
async function load(file: string): Promise<unknown> {
  const mod: unknown = await import(pathToFileURL(file).href);
  return mod;
}

async function recordVSCode(): Promise<void> {
  const latest: unknown = JSON.parse(execFileSync('gh', ['api', 'repos/microsoft/vscode/releases/latest']).toString('utf8'));
  const tag = text(at(latest, 'the latest release of VS Code', 'tag_name'), 'the tag of the latest release of VS Code');
  const base = 'extensions/markdown-language-features';
  const slugify = vscodeFile(`${base}/src/slugify.ts`, tag);
  const engine = vscodeFile(`${base}/src/markdownEngine.ts`, tag);
  const lock: unknown = JSON.parse(vscodeFile(`${base}/package-lock.json`, tag));
  const mdVersion = text(at(lock, `${base}/package-lock.json`, 'packages', 'node_modules/markdown-it', 'version'), 'the version of markdown-it');
  const nodeVersion = vscodeFile('.nvmrc', tag).trim();
  const rootLock: unknown = JSON.parse(vscodeFile('package-lock.json', tag));
  const ts = at(rootLock, 'package-lock.json of VS Code', 'packages', 'node_modules/typescript');
  const tsName = isRecord(ts) && typeof ts.name === 'string' ? ts.name : 'typescript';
  const tsVersion = text(at(ts, 'the typescript of VS Code', 'version'), 'the version of the typescript of VS Code');
  // The markdown-math extension, which VS Code ships with markdown.math.enabled true by default, contributes
  // @vscode/markdown-it-katex to the engine; its options are those of its extension.ts.
  const mathBase = 'extensions/markdown-math';
  const mathExtension = vscodeFile(`${mathBase}/src/extension.ts`, tag);
  const mathLock: unknown = JSON.parse(vscodeFile(`${mathBase}/package-lock.json`, tag));
  const katexPluginVersion = text(at(mathLock, `${mathBase}/package-lock.json`, 'packages', 'node_modules/@vscode/markdown-it-katex', 'version'), 'the version of @vscode/markdown-it-katex');
  const katexVersion = text(at(mathLock, `${mathBase}/package-lock.json`, 'packages', 'node_modules/katex', 'version'), 'the version of katex');
  const mathOptions = katexOptions(mathExtension);
  const mathPackage: unknown = JSON.parse(vscodeFile(`${mathBase}/package.json`, tag));
  const configuration = at(mathPackage, `${mathBase}/package.json`, 'contributes', 'configuration');
  if (!Array.isArray(configuration)) throw new Error(`${mathBase}/package.json contributes no configuration list`);
  const enabled = configuration.flatMap((c: unknown) => isRecord(c) && isRecord(c.properties) ? Object.entries(c.properties) : []).find(([k]) => k === 'markdown.math.enabled');
  if (enabled === undefined) throw new Error(`${mathBase}/package.json declares no markdown.math.enabled`);
  const mathEnabled = at(enabled[1], 'markdown.math.enabled', 'default');
  if (mathEnabled !== true) throw new Error(`markdown.math.enabled defaults to ${String(mathEnabled)}`);
  const work = mkdtempSync(path.join(tmpdir(), 'record-slugs-'));
  try {
    execFileSync('npm', ['install', '--prefix', work, '--no-save', '--ignore-scripts', '--no-audit', '--no-fund', `markdown-it@${mdVersion}`, `typescript@npm:${tsName}@${tsVersion}`, `@vscode/markdown-it-katex@${katexPluginVersion}`, `katex@${katexVersion}`], { stdio: 'ignore' });
    const typescript = at(await load(path.join(work, 'node_modules', 'typescript', 'lib', 'typescript.js')), 'typescript.js', 'default');
    if (!isTranspiler(typescript)) throw new Error('typescript.js has no transpileModule');
    // The TypeScript sources compiled as VS Code compiles them, by the TypeScript its package-lock.json pins.
    const compile = (src: string): string => typescript.transpileModule(src, { compilerOptions: { target: typescript.ScriptTarget.ES2022, module: typescript.ModuleKind.ESNext } }).outputText;
    writeFileSync(path.join(work, 'slugify.mjs'), compile(slugify));
    const method = methodSource(engine, '#tokenToPlainText(token: MarkdownIt.Token): string {');
    writeFileSync(path.join(work, 'plain.mjs'), compile(`export class Engine {\n${method}\nplain(token: any): string { return this.#tokenToPlainText(token); }\n}\n`));
    const githubSlugifier = at(await load(path.join(work, 'slugify.mjs')), 'slugify.mjs', 'githubSlugifier');
    if (!isSlugifier(githubSlugifier)) throw new Error('slugify.ts has no githubSlugifier with createBuilder');
    const Engine = at(await load(path.join(work, 'plain.mjs')), 'plain.mjs', 'Engine');
    if (!isEngine(Engine)) throw new Error('plain.mjs has no Engine class');
    const markdownit = at(await load(path.join(work, 'node_modules', 'markdown-it', 'index.mjs')), 'markdown-it', 'default');
    if (!isMarkdownFactory(markdownit)) throw new Error('markdown-it exports no function');
    const katexPlugin = at(await load(path.join(work, 'node_modules', '@vscode', 'markdown-it-katex', 'dist', 'index.js')), '@vscode/markdown-it-katex', 'default', 'default');
    let md = markdownit({ html: true, breaks: false, linkify: true, typographer: false });
    md.linkify.set({ fuzzyLink: false });
    md = md.use(katexPlugin, mathOptions);
    const plain = new Engine();
    // The plain text of the inline token after the heading_open token at i.
    const inlineAfter = (tokens: MarkdownToken[], i: number): MarkdownToken => {
      const inline = tokens[i + 1];
      if (inline === undefined) throw new Error('a heading_open token ends the token stream');
      return inline;
    };
    // The heading ids of #addNamedHeaders: the slug builder of the render, fed the plain text of each heading's
    // inline token.
    const render: Render = source => {
      const builder = githubSlugifier.createBuilder();
      const tokens = md.parse(source, {});
      const ids: string[] = [];
      tokens.forEach((t, i) => {
        if (t.type === 'heading_open') ids.push(builder.add(plain.plain(inlineAfter(tokens, i))).value);
      });
      return ids;
    };
    // The id attribute of every heading element of the preview's HTML: #addNamedHeaders sets it on each heading_open
    // token; raw HTML is written as it stands.
    const renderHtml: Render = source => {
      const builder = githubSlugifier.createBuilder();
      const tokens = md.parse(source, {});
      tokens.forEach((t, i) => {
        if (t.type === 'heading_open') t.attrSet('id', builder.add(plain.plain(inlineAfter(tokens, i))).value);
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
      'Re-record with make slug-vectors (scripts/record-slugs.mts).',
    ];
    const ref = codePointIds('ref', render, 'VS Code');
    const raw = codePointIds('raw', render, 'VS Code');
    writeFileSync(path.join(vectorDir, 'vscode-codepoints.txt'), codePointFile(header, ref, raw));
    const file = {
      viewer: 'VS Code',
      recorded: today(),
      request: `VS Code ${tag}: ${base}/src/slugify.ts and markdownEngine.ts #tokenToPlainText, markdown-it ${mdVersion}, @vscode/markdown-it-katex ${katexPluginVersion} with ${JSON.stringify(mathOptions)}, node ${process.versions.node}`,
      rerecord: 'make slug-vectors (scripts/record-slugs.mts)',
      cases: structuralCases.map(([name, markdown]) => ({ name, markdown, ids: render(markdown + '\n') })),
      rawHeadings: rawHeadingCases.map(([name, markdown]) => ({ name, markdown, ids: renderHtml(markdown + '\n') })),
    };
    writeFileSync(path.join(vectorDir, 'vscode-headings.json'), JSON.stringify(file, null, 1) + '\n');
    // The slugify.ts that was run, whose expression TestVSRemovalIsVSCodeRegex reads, with VS Code's MIT license.
    writeFileSync(path.join(vectorDir, 'vscode-slugify.ts'), withReason(slugify));
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
export function glfmArgs(src: string): string[] {
  const body = /OPTIONS = \{([^}]*)\}\.freeze/.exec(src);
  if (!body) throw new Error('no OPTIONS in glfm_markdown.rb');
  const chars = /REFERENCE_CHARS = %w\[([^\]]*)\]\.freeze/.exec(src);
  const args: string[] = [];
  for (const line of (body[1] ?? '').split('\n').map(l => l.trim().replace(/,$/, '')).filter(Boolean)) {
    const m = /^(\w+): (.+)$/.exec(line);
    const [, name, value] = m ?? [];
    if (name === undefined || value === undefined) throw new Error(`an OPTIONS line the recorder does not read: ${line}`);
    const flag = `--${name.replaceAll('_', '-')}`;
    if (value === 'true') args.push(flag);
    else if (value === 'false') continue;
    else if (name === 'header_id_prefix' && value === 'Banzai::Renderer::USER_CONTENT_ID_PREFIX') args.push(flag, 'user-content-');
    else if (name === 'only_escape_chars' && value === 'REFERENCE_CHARS' && chars?.[1] !== undefined) args.push(flag, chars[1].split(/\s+/).join(''));
    else throw new Error(`an OPTIONS value the recorder does not read: ${line}`);
  }
  return args;
}

/**
 * The heading ids of the gitlab-glfm-markdown rendering html, one per heading element in order: the id of a heading
 * comrak renders, which carries data-sourcepos, without user-content-; for raw HTML, which comrak writes as it stands
 * and gives no id of its own, the id attribute as written, or null.
 */
export function gitlabIds(html: string): (string | null)[] {
  return [...html.matchAll(/<h[1-6]((?=[\s>/])[^>]*)>/gi)].map(m => {
    const attrs = m[1] ?? '';
    if (!/\sdata-sourcepos="/.test(attrs)) {
      const written = /\sid="([^"]*)"/i.exec(attrs);
      return written ? unescapeAttr(written[1] ?? '') : null;
    }
    const id = /\sid="([^"]*)"/.exec(attrs);
    if (!id) throw new Error(`a heading of comrak without an id: ${m[0]}`);
    const v = unescapeAttr(id[1] ?? '');
    if (!v.startsWith('user-content-')) throw new Error(`a heading id without user-content-: ${v}`);
    return v.slice('user-content-'.length);
  });
}

/** The text of a URL, or an error naming it. */
async function fetchText(url: string): Promise<string> {
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
async function buildGlfm(version: string, sha256: string, work: string): Promise<string> {
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
  // A section that the gem's Cargo.toml does not declare has no lines.
  const section = (name: string): string => new RegExp(`^\\[${name}\\]\\n([^]*?)(?=^\\[|(?![^]))`, 'm').exec(toml)?.[1] ?? '';
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

async function recordGitLab(): Promise<void> {
  const project = 'https://gitlab.com/api/v4/projects/gitlab-org%2Fgitlab';
  const master: unknown = JSON.parse(await fetchText(`${project}/repository/commits/master`));
  const commit = text(at(master, 'the master commit of GitLab', 'id'), 'the id of the master commit of GitLab');
  const raw = (file: string): Promise<string> => fetchText(`https://gitlab.com/gitlab-org/gitlab/-/raw/${commit}/${file}`);
  const lock = /^ {2}gitlab-glfm-markdown \(([0-9.]+)\) sha256=([0-9a-f]{64})$/m.exec(await raw('Gemfile.lock'));
  const [, version, sha256] = lock ?? [];
  if (version === undefined || sha256 === undefined) throw new Error("no gitlab-glfm-markdown checksum in GitLab's Gemfile.lock");
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
    const render: Render = source => gitlabIds(execFileSync(bin, args, { input: source, maxBuffer: 1 << 30 }).toString('utf8'));
    const options = args.join(' ');
    const header = [
      "GitLab's heading ids per code point (spec 10, Detail headings and anchors), the vectors of S_GL.",
      `Recorded ${today()} by running gitlab-glfm-markdown ${version}, the version and sha256 GitLab's Gemfile.lock`,
      `pins at commit ${commit}, with the OPTIONS of its ${engineFile}:`,
      options,
      'Each heading id is user-content- and the id here. Re-record with make slug-vectors (scripts/record-slugs.mts).',
    ];
    const ref = codePointIds('ref', render, 'GitLab');
    const rawIds = codePointIds('raw', render, 'GitLab');
    writeFileSync(path.join(vectorDir, 'gitlab-codepoints.txt'), codePointFile(header, ref, rawIds));
    const file = {
      viewer: 'GitLab',
      recorded: today(),
      request: `gitlab-glfm-markdown ${version} (GitLab ${commit}), ${options}; ids without user-content-`,
      rerecord: 'make slug-vectors (scripts/record-slugs.mts)',
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
