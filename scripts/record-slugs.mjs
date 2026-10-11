// The recorder of the slug vectors: the heading ids that GitHub and VS Code render, observed from the viewers themselves,
// which store's tests require S_GH and S_VS to reproduce (spec 10, Detail headings and anchors). It writes, under
// store/testdata/slug, for each viewer a code point file, the id of a heading holding each code point but the
// surrogates, and a heading file, the ids of the structural cases below; for GitHub also the id and name attributes it
// renders for raw HTML. GitHub is asked through `gh api` (POST /markdown, mode markdown, the mode that renders heading
// anchors); VS Code is run: the latest release's slugify.ts and the #tokenToPlainText method of markdownEngine.ts of
// extensions/markdown-language-features, at the release tag, with the markdown-it version its package-lock.json pins,
// configured as the preview configures it by default. It needs the network, an authenticated gh and npm; `make
// slug-vectors` runs it.
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
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
  if (cp === 0x0a || cp === 0x0d || cp === 0) return { ref, raw: null };
  const ch = String.fromCodePoint(cp);
  return { ref, raw: isASCIIPunct(cp) ? `\\${ch}` : ch };
}

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
    if (cp === 0x20 || cp === 0x2d) headings.push({ kind: 'single', cps: [cp], line: `# s${cp.toString(16)}p${f}q` });
    else grouped.push([cp, f]);
  }
  for (let i = 0; i < grouped.length; i += groupSize) {
    const part = grouped.slice(i, i + groupSize);
    headings.push({ kind: 'group', cps: part.map(([cp]) => cp), line: `# g${headings.length}-${part.map(([, f]) => f).join('-')}-` });
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
  return heading.cps.map(cp => ({ kind: 'single', cps: [cp], line: `# s${cp.toString(16)}p${forms(cp)[form]}q` }));
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
];

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
 * The heading ids of GitHub's rendering html, one per heading element in order: the id of the anchor that follows the
 * heading, without user-content-; the empty string for an anchor whose id is empty, which GitHub writes without the
 * prefix; null for a heading that GitHub gives no anchor.
 */
export function githubIds(html) {
  return html.split(/(?=<h[1-6][\s>])/).filter(part => /^<h[1-6][\s>]/.test(part)).map(part => {
    const m = part.match(/^[^]*?<\/h[1-6]>(?:<a id="([^"]*)" class="anchor")?/);
    if (!m) throw new Error(`a heading that does not close: ${part.slice(0, 80)}`);
    if (m[1] === undefined) return null;
    const id = unescapeAttr(m[1]);
    if (id === '') return '';
    if (!id.startsWith('user-content-')) throw new Error(`a heading anchor without user-content-: ${id}`);
    return id.slice('user-content-'.length);
  });
}

/** The id and name attributes of GitHub's rendering html other than its heading anchors, in order. */
export function githubAttributes(html) {
  return [...html.matchAll(/\s(id|name)="([^"]*)"(?! class="anchor")/g)].map(m => ({ attr: m[1], value: unescapeAttr(m[2]) }));
}

/** The largest document sent to GitHub in one request, in bytes; the API renders at most 400 KB. */
const requestLimit = 390_000;

/** GitHub's rendering of a document, by POST /markdown in mode markdown, through gh. */
function githubRender(text) {
  return execFileSync('gh', ['api', '-X', 'POST', '/markdown', '--input', '-'], {
    input: JSON.stringify({ text, mode: 'markdown' }),
    maxBuffer: 1 << 30,
  }).toString('utf8');
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
function codePointFile(header, ref, raw) {
  const head = [
    ...header.map(l => `# ${l}`),
    '#',
    '# Each line is lo..hi ; ref ; raw: the code points lo to hi (the surrogates skipped), and the id of a heading',
    '# holding the code point between two hyphens, without them: ref when the heading holds it as its numeric',
    '# character reference &#xN;, raw when it holds the character itself (an ASCII punctuation character',
    '# backslash-escaped). drop: the id holds nothing; keep: the character; +N or -N: the one character N',
    '# (hexadecimal) code points away; else the code points of the id; none: no such heading (a line ending,',
    '# or U+0000, whose raw form GitHub renders no document with).',
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
  const cases = structuralCases.map(([name, markdown]) => ({ name, markdown, ids: render(markdown + '\n') }));
  const attributes = attributeCases.map(([name, markdown]) => ({ name, markdown, values: githubAttributes(githubRender(markdown + '\n')) }));
  const file = {
    viewer: 'GitHub',
    recorded: today(),
    request: 'POST https://api.github.com/markdown, mode "markdown"; ids without user-content-',
    rerecord: 'make slug-vectors (scripts/record-slugs.mjs)',
    cases,
    attributes,
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

async function recordVSCode() {
  const tag = JSON.parse(execFileSync('gh', ['api', 'repos/microsoft/vscode/releases/latest']).toString('utf8')).tag_name;
  const base = 'extensions/markdown-language-features';
  const slugify = vscodeFile(`${base}/src/slugify.ts`, tag);
  const engine = vscodeFile(`${base}/src/markdownEngine.ts`, tag);
  const lock = JSON.parse(vscodeFile(`${base}/package-lock.json`, tag));
  const mdVersion = lock.packages['node_modules/markdown-it'].version;
  const nodeVersion = vscodeFile('.nvmrc', tag).trim();
  const ts = JSON.parse(vscodeFile('package-lock.json', tag)).packages['node_modules/typescript'];
  const work = mkdtempSync(path.join(tmpdir(), 'record-slugs-'));
  try {
    execFileSync('npm', ['install', '--prefix', work, '--no-save', '--ignore-scripts', '--no-audit', '--no-fund', `markdown-it@${mdVersion}`, `typescript@npm:${ts.name ?? 'typescript'}@${ts.version}`], { stdio: 'ignore' });
    const typescript = (await import(pathToFileURL(path.join(work, 'node_modules', 'typescript', 'lib', 'typescript.js')).href)).default;
    // The TypeScript sources compiled as VS Code compiles them, by the TypeScript its package-lock.json pins.
    const compile = src => typescript.transpileModule(src, { compilerOptions: { target: typescript.ScriptTarget.ES2022, module: typescript.ModuleKind.ESNext } }).outputText;
    writeFileSync(path.join(work, 'slugify.mjs'), compile(slugify));
    const method = methodSource(engine, '#tokenToPlainText(token: MarkdownIt.Token): string {');
    writeFileSync(path.join(work, 'plain.mjs'), compile(`export class Engine {\n${method}\nplain(token: any): string { return this.#tokenToPlainText(token); }\n}\n`));
    const { githubSlugifier } = await import(pathToFileURL(path.join(work, 'slugify.mjs')).href);
    const { Engine } = await import(pathToFileURL(path.join(work, 'plain.mjs')).href);
    const markdownit = (await import(pathToFileURL(path.join(work, 'node_modules', 'markdown-it', 'index.mjs')).href)).default;
    const md = markdownit({ html: true, breaks: false, linkify: true, typographer: false });
    md.linkify.set({ fuzzyLink: false });
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
    const header = [
      "VS Code's heading ids per code point (spec 10, Detail headings and anchors), the vectors of S_VS.",
      `Recorded ${today()} by running VS Code ${tag}'s own code: ${base}/src/slugify.ts (githubSlugifier) and`,
      `the #tokenToPlainText method of src/markdownEngine.ts, with markdown-it ${mdVersion} as its package-lock.json`,
      `pins, configured as the preview's defaults (html, linkify without fuzzy links, no breaks, no typographer);`,
      `node ${process.versions.node} (Unicode ${process.versions.unicode}), VS Code ${tag} names node ${nodeVersion}.`,
      'Re-record with make slug-vectors (scripts/record-slugs.mjs).',
    ];
    const ref = codePointIds('ref', render, 'VS Code');
    const raw = codePointIds('raw', render, 'VS Code');
    writeFileSync(path.join(vectorDir, 'vscode-codepoints.txt'), codePointFile(header, ref, raw));
    const file = {
      viewer: 'VS Code',
      recorded: today(),
      request: `VS Code ${tag}: ${base}/src/slugify.ts and markdownEngine.ts #tokenToPlainText, markdown-it ${mdVersion}, node ${process.versions.node}`,
      rerecord: 'make slug-vectors (scripts/record-slugs.mjs)',
      cases: structuralCases.map(([name, markdown]) => ({ name, markdown, ids: render(markdown + '\n') })),
    };
    writeFileSync(path.join(vectorDir, 'vscode-headings.json'), JSON.stringify(file, null, 1) + '\n');
    // The slugify.ts that was run, whose expression TestVSRemovalIsVSCodeRegex reads, with VS Code's MIT license.
    writeFileSync(path.join(vectorDir, 'vscode-slugify.ts'), slugify);
    writeFileSync(path.join(vectorDir, 'vscode-LICENSE.txt'), vscodeFile('LICENSE.txt', tag));
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  mkdirSync(vectorDir, { recursive: true });
  const which = process.argv.slice(2);
  if (which.length === 0 || which.includes('github')) recordGitHub();
  if (which.length === 0 || which.includes('vscode')) await recordVSCode();
  log(`wrote ${vectorDir}`);
}
