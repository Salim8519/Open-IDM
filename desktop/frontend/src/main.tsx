import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./styles.css";

// Paint the right theme before settings arrive: the last one used, or the default
// Field Guide edition for the system's day/night. App keeps data-theme current after.
try {
  const last = localStorage.getItem("peregrine.theme");
  const dark = window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? true;
  document.documentElement.dataset.theme = last || (dark ? "field-night" : "field-day");
} catch {
  // storage unavailable: the Classic tokens show until settings load
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
