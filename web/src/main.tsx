import React from "react";
import ReactDOM from "react-dom/client";
import { Startup } from "./Startup";
import "./styles/v4-tokens.css";
import "./styles/brand.css";
import "./v1-tokens.css";
import "./v1-shell.css";
import "./v1-page.css";
import "./v1-forms.css";
import "./shell-v2.css";
import "./shell-v2-pages.css";
/* V4 shell must come last so its topbar / rail / main / action overrides
 * win against the legacy shell-v2 selectors. */
import "./styles/v4-shell.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <Startup />
  </React.StrictMode>,
);
