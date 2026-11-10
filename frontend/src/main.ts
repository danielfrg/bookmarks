import { mount } from "svelte";
import { initTheme, setThemeMode, setThemeName } from "@kenn-io/kit-ui";
import "@kenn-io/kit-ui/theme.css";
import "./theme.css";
import App from "./App.svelte";
import "./app.css";
import "./components.css";

initTheme({ storageKey: "bookmarks-theme" });
// The design system's theme lives under data-kit-theme="linear"; initTheme()
// removes that attribute when no theme name is stored, so set it again.
setThemeName("linear");
// Default to charcoal while respecting explicit light and system preferences.
try {
  if (!localStorage.getItem("bookmarks-theme")) setThemeMode("dark");
} catch {
  setThemeMode("dark");
}
mount(App, { target: document.getElementById("app")! });
