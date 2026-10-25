// The one browser script of the board example. Without `<meta name="hyper-api">` the page was
// rendered by the server (SSR); with it the page is a static shell that renders in the browser (CSR).
import htmx from 'htmx.org';
import { checkManifest, createApplication, Hyper } from '@polyspec/hyper-client';
import manifest from '../app/app.json';
import index from '@polyspec/hyper-client/templates-index';

const app = createApplication(checkManifest(manifest), index, async (url) => {
  const response = await fetch(url);
  if (!response.ok) throw new Error(`template ${url}: status ${response.status}`);
  // The decoder checks the fetched JSON against the AST schema; import() makes it a chunk that a server-rendered page,
  // which loads no template, never loads (HYC-12, HYC-16).
  const { decodeTemplate } = await import('@polyspec/template/decode');
  return decodeTemplate(await response.json());
});
const basePath = document.querySelector('meta[name="hyper-api"]')?.getAttribute('content') ?? null;
const hyper = new Hyper<Element>(app, htmx, { basePath: basePath ?? '' });
window.hyper = hyper;

htmx.registerExtension('hyper', hyper.extension());

// HYC-5: a click on an element with hy-set changes region data and renders the region without a request.
document.addEventListener('click', (event) => {
  const element = event.target instanceof Element ? event.target.closest('[hy-set]') : null;
  if (!element) return;
  event.preventDefault();
  event.stopPropagation();
  // A failure leaves the data unchanged and marks the region with hy-error (HYC-14).
  hyper.setFrom(element).then(report, (error: unknown) => console.error(error));
}, true);

// The comparison page embeds both modes in frames and compares them after htmx has processed a new
// body and after every swap. A frame sends only the SHA-256 of its body, because the body holds the
// CSRF token, and only to the origin of the page that embeds it. Every response masks the token anew
// (HY-24), so the digest reads the body with each masked token, 128 hexadecimal digits, as <csrf>. A client-rendered
// document never embeds its data (HY-31), so the digest reads the body without the embedded data of the post page.
const mode = basePath === null ? 'ssr' : 'csr';
const parentOrigin = window.parent !== window && document.referrer !== '' ? new URL(document.referrer).origin : null;
async function report(): Promise<void> {
  if (parentOrigin === null) return;
  const body = document.body.cloneNode(true);
  if (!(body instanceof HTMLElement)) throw new Error('the copy of the body is no element');
  body.querySelector('#hy-data')?.remove();
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(body.innerHTML.replace(/\b[0-9a-f]{128}\b/g, '<csrf>')));
  const hash = Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
  window.parent.postMessage({ type: 'hyper-body', mode, path: location.pathname + location.search, body: hash }, parentOrigin);
}
document.addEventListener('htmx:after:settle', () => void report());
document.addEventListener('htmx:after:process', (event) => {
  if (event.target === document.body) void report();
});

// HYC-8, HYC-4, HYC-7: a client-rendered page renders its document; a server-rendered page holds its embedded data, or
// obtains the data when it first needs it.
void hyper.open();
