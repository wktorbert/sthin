import { invoke } from "@tauri-apps/api/core";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { startApp } from "./store";
import { followSystemTheme } from "./theme";
import { bindTray } from "./tray";
import "./index.css";

followSystemTheme();

// Cmd+Q / Ctrl+Q quits for real; the close button only hides the window.
window.addEventListener("keydown", (e) => {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "q") {
    e.preventDefault();
    void invoke("quit_app");
  }
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

void startApp().then(bindTray);
