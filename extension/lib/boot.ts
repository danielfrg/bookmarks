import { mount, type Component } from "svelte";
import { initTheme, setThemeMode } from "@kenn-io/kit-ui";
import "@kenn-io/kit-ui/theme.css";
import "../src/theme.css";
import "../src/app.css";
import "../src/components.css";

const THEME_KEY = "bookmarks-ext-theme";

/** Applies the shared theme (dark by default) and mounts a page. */
export function boot(App: Component) {
  initTheme({ storageKey: THEME_KEY });
  try {
    if (!localStorage.getItem(THEME_KEY)) setThemeMode("dark");
  } catch {
    setThemeMode("dark");
  }
  mount(App, { target: document.getElementById("app")! });
}
