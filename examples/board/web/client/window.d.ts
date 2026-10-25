// The globals of the board page: main.ts sets the Hyper instance as hyper, and htmx.esm.js of htmx.org sets htmx. The
// end-to-end tests and the browser benchmark read them in the page.
import type { Hyper } from '@polyspec/hyper-client';
import type { Htmx } from 'htmx.org';

declare global {
  interface Window {
    hyper: Hyper;
    htmx: Htmx;
  }
}
