// Minimal types for the parts of single-file-core (plain ESM, no typings) we use.
declare module "single-file-core/single-file.js" {
  export interface InitOptions {
    fetch?: (url: string, options?: object) => Promise<unknown>;
    frameFetch?: (url: string, options?: object) => Promise<unknown>;
  }
  export interface PageData {
    content: string;
    title: string;
    filename?: string;
  }
  export function init(initOptions: InitOptions): void;
  export function getPageData(
    options: Record<string, unknown>,
    initOptions: InitOptions,
    doc: Document,
    win: Window,
  ): Promise<PageData>;
}
