import { mount, type Component } from "svelte";
import { initTheme, setThemeMode, setThemeName } from "@kenn-io/kit-ui";
import "@kenn-io/kit-ui/theme.css";
import "../src/theme.css";
import "../src/app.css";
import "../src/components.css";

const THEME_KEY = "bookmarks-ext-theme";

/** Applies the shared homelab theme (dark by default) and mounts a page. */
export function boot(App: Component) {
  initTheme({ storageKey: THEME_KEY });
  // The design system's theme lives under data-kit-theme="linear"; initTheme()
  // removes that attribute when no theme name is stored, so set it again.
  setThemeName("linear");
  try {
    if (!localStorage.getItem(THEME_KEY)) setThemeMode("dark");
  } catch {
    setThemeMode("dark");
  }
  mount(App, { target: document.getElementById("app")! });
}
