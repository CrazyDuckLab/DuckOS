// Nodes UI security closure regressions: the staged-rotation guidance
// must recommend ONLY the supported protected-stdin contract, must
// never render literal credential flags or secret values, must clear
// the credential variable on success AND failure, must label the
// shell-specific example correctly, and must use copy-paste-safe
// shell syntax. The Stage → Pull → Confirm → Recover flow semantics
// must stay intact in all four locales.
import { describe, expect, it } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { XrayCenterPage } from "../xray/center";
import type { FetchResult } from "../api/client";
import type { PageMeta } from "../pages/registry";

const pageStub = {
  id: "P050",
  name: "Xray Center",
  route: "/tunnels/xray/profiles",
  question: "",
} as unknown as PageMeta;

type NodeFixture = {
  id: string;
  name: string;
  status: string;
  version?: string;
  capabilities?: string[];
  lastSeenAt?: string;
  lastError?: string;
};

function guidanceClient(
  nodes: NodeFixture[],
  overrides: { rotateFails?: boolean } = {},
) {
  const client = {
    fetchXrayServers: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayUsers: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayCenterStats: () =>
      Promise.resolve({
        ok: true,
        data: { stale: false, ledger: {} },
        age: 0,
      } as FetchResult<unknown>),
    fetchXrayDiagnostics: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayNodes: () =>
      Promise.resolve({ ok: true, data: nodes, age: 0 } as FetchResult<
        NodeFixture[]
      >),
    fetchXrayGroups: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayStorage: () =>
      Promise.resolve({
        ok: true,
        data: { domains: {} },
        age: 0,
      } as FetchResult<unknown>),
    xrayIssueEnrollToken: () =>
      Promise.resolve({
        ok: true,
        data: { token: "TOKEN-VALUE" },
        age: 0,
      } as FetchResult<unknown>),
    xrayStageRotation: () => {
      if (overrides.rotateFails) {
        return Promise.reject(new Error("503"));
      }
      return Promise.resolve({
        ok: true,
        data: { credential: "NEW-CRED-VALUE", state: "PENDING" },
        age: 0,
      } as FetchResult<unknown>);
    },
    fetchXrayRotationState: () =>
      Promise.resolve({
        ok: true,
        data: { state: "PENDING" },
        age: 0,
      } as FetchResult<unknown>),
    xraySuspendNode: () =>
      Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>),
    xrayResumeNode: () =>
      Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>),
    xrayRevokeNode: () =>
      Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>),
    xrayReconcileStorage: () =>
      Promise.resolve({
        ok: true,
        data: { result: "recovered" },
        age: 0,
      } as FetchResult<unknown>),
  };
  return client as never;
}

const activeNode: NodeFixture = {
  id: "n-1",
  name: "edge-1",
  status: "active",
  version: "26.3.27",
  capabilities: ["vless", "reality"],
  lastSeenAt: "2026-10-20T10:00:00Z",
};

const locales = ["en", "ru", "de", "es"] as const;
// Button labels: ru is localized; de/es intentionally inherit the EN
// label (only the guidance text is overridden per-locale), matching
// the unchanged independent audit test.
const rotateLabels: Record<(typeof locales)[number], string> = {
  en: "Rotate credential",
  ru: "Ротация креденшала",
  de: "Rotate credential",
  es: "Rotate credential",
};

async function renderedGuidance(locale: (typeof locales)[number]) {
  const client = guidanceClient([activeNode]);
  const { unmount } = render(
    <XrayCenterPage
      page={pageStub}
      bundle={{} as never}
      client={client}
      locale={locale}
    />,
  );
  await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
  fireEvent.click(screen.getByText(rotateLabels[locale]));
  await waitFor(() => expect(screen.getByText("NEW-CRED-VALUE")).toBeTruthy());
  const guidance = screen.getByText(/pull-rotation/).textContent ?? "";
  unmount();
  return guidance;
}


// The rendered command is identical across locales except the
// (localized) interactive prompt strings.
const readPrompts: Record<(typeof locales)[number], string> = {
  en: "Current node credential: ",
  ru: "Текущий креденшал узла: ",
  de: "Aktuelles Knoten-Credential: ",
  es: "Credencial actual del nodo: ",
};
const bashLabel: Record<(typeof locales)[number], string> = {
  en: ", in Bash (",
  ru: ", в Bash (",
  de: ", in Bash (",
  es: ", en Bash (",
};
const zshRead: Record<(typeof locales)[number], string> = {
  en: "read -s 'NODE_CRED?Current node credential: '",
  ru: "read -s 'NODE_CRED?Текущий креденшал узла: '",
  de: "read -s 'NODE_CRED?Aktuelles Knoten-Credential: '",
  es: "read -s 'NODE_CRED?Credencial actual del nodo: '",
};
const bashTemplate = (prompt: string) =>
  "read -s -p '" +
  prompt +
  "' NODE_CRED && " +
  'printf \'%s\\n\' "$NODE_CRED" | duck xray nodes pull-rotation ' +
  "--id '<id>' --credential-stdin; unset NODE_CRED";

describe("Rotation guidance uses the protected stdin contract", () => {
  for (const locale of locales) {
    it(`safe, robust stdin guidance is rendered in ${locale}`, async () => {
      const guidance = await renderedGuidance(locale);
      // The supported protected-stdin contract is present…
      expect(guidance).toContain("--credential-stdin");
      // …and no literal credential flag form is recommended.
      expect(guidance).not.toMatch(/--credential(?:\s|=)/);
      // The command names the real operation and uses the
      // copy-paste-safe quoted id placeholder.
      expect(guidance).toContain("duck xray nodes pull-rotation");
      expect(guidance).toContain("--id '<id>'");
      // Cleanup is guaranteed on success AND failure (a plain `;`
      // sequence, not `&& unset`).
      expect(guidance).toContain("credential-stdin; unset NODE_CRED");
      expect(guidance).not.toContain("&& unset NODE_CRED");
      // The Bash-specific example is labeled, and the Zsh read variant
      // is provided separately.
      expect(guidance).toContain(bashLabel[locale]);
      expect(guidance).toContain(zshRead[locale]);
      // Interrupted input has a documented cleanup path.
      expect(guidance).toMatch(/interrupted|прерван|abgebrochen|interrumpe/);
      // No secret value leaks into the generated command text and no
      // unsafe shortcuts are recommended.
      expect(guidance).not.toContain("NEW-CRED-VALUE");
      expect(guidance).not.toContain("TOKEN-VALUE");
      expect(guidance).not.toMatch(/echo\s+['"]?[A-Za-z0-9_-]*SECRET/i);
      // Staging alone is not presented as a completed rotation.
      expect(guidance).toMatch(/PENDING/);
    });
  }

  it("staging alone does not claim completion in any locale", async () => {
    for (const locale of locales) {
      const guidance = await renderedGuidance(locale);
      expect(guidance).toMatch(/PENDING/);
    }
  });

  it("rotation failure keeps the state untouched message", async () => {
    const client = guidanceClient([activeNode], { rotateFails: true });
    render(
      <XrayCenterPage
        page={pageStub}
        bundle={{} as never}
        client={client}
        locale="en"
      />,
    );
    await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
    fireEvent.click(screen.getByText("Rotate credential"));
    await waitFor(() =>
      expect(screen.getByText(/Rotation failed/)).toBeTruthy(),
    );
    expect(screen.queryByText(/pull-rotation/)).toBeNull();
  });
});

describe("Rendered Bash guidance is valid, working shell", () => {

    "read -s -p '" +
    prompt +
    "' NODE_CRED && " +
    'printf \'%s\\n\' "$NODE_CRED" | duck xray nodes pull-rotation ' +
    "--id '<id>' --credential-stdin; unset NODE_CRED";

  it("the Bash command passes bash -n in every locale variant", async () => {
    for (const locale of locales) {
      const guidance = await renderedGuidance(locale);
      const template = bashTemplate(readPrompts[locale]);
      expect(guidance).toContain(template);
      // Syntax check: the quoted '<id>' placeholder keeps the line
      // valid shell even before substitution.
      execFileSync("/bin/bash", ["-n", "-c", template]);
      // After substituting a real (quoted) node id it stays valid.
      execFileSync("/bin/bash", [
        "-n",
        "-c",
        template.replace("'<id>'", "'n-12345'"),
      ]);
    }
  });

  it("the Zsh read variant passes zsh -n in every locale", () => {
    for (const locale of locales) {
      execFileSync("/bin/zsh", ["-n", "-c", zshRead[locale]]);
    }
  });

  const runWithStubDuck = (duckExit: number, id: string) => {
    const dir = mkdtempSync(join(tmpdir(), "guidance-shell-"));
    const stub = join(dir, "duck");
    // The stub records stdin (the piped credential) and its argv, then
    // exits with the requested code.
    writeFileSync(
      stub,
      "#!/bin/bash\ncat > " +
        JSON.stringify(join(dir, "stdin.txt")) +
        '\nprintf \'%s\\n\' "$*" > ' +
        JSON.stringify(join(dir, "argv.txt")) +
        "\nexit " +
        duckExit +
        "\n",
      { mode: 0o755 },
    );
    const script = bashTemplate(readPrompts.en).replace("'<id>'", `'${id}'`);
    const out = execFileSync(
      "/bin/bash",
      ["-c", 'PATH=' + JSON.stringify(dir) + ':$PATH; ' + script + '; printf \'%s\' "${NODE_CRED+SET}"'],
      { input: "SECRET-INPUT-VALUE\n" },
    );
    return { dir, out };
  };

  it("clears the credential variable and pipes the secret on success", () => {
    const { dir, out } = runWithStubDuck(0, "n-12345");
    // NODE_CRED is unset after the run (the trailing probe printed
    // nothing).
    expect(out.toString()).toBe("");
    expect(readFileSync(join(dir, "stdin.txt"), "utf8")).toContain(
      "SECRET-INPUT-VALUE",
    );
    // The secret never appears in the stub's argv: it travels only
    // through stdin.
    expect(readFileSync(join(dir, "argv.txt"), "utf8")).not.toContain(
      "SECRET-INPUT-VALUE",
    );
    rmSync(dir, { recursive: true, force: true });
  });

  it("clears the credential variable even when the CLI fails", () => {
    const { dir, out } = runWithStubDuck(3, "n-12345");
    expect(out.toString()).toBe("");
    expect(readFileSync(join(dir, "stdin.txt"), "utf8")).toContain(
      "SECRET-INPUT-VALUE",
    );
    rmSync(dir, { recursive: true, force: true });
  });
});
