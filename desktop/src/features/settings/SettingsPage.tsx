import { useEffect, useState } from "react";
import { ChevronRight, CircleAlert, TriangleAlert } from "lucide-react";
import { appVersion, runDiagnostics } from "../../lib/api/daemon";
import type { CloseBehavior, LoadedSettings, Settings, Theme } from "../../lib/api/types";
import { useDaemon } from "../../app/sync/useDaemon";
import { keyringLabel } from "../diagnostics/checks";
import { Alert, AlertDescription, AlertTitle } from "@/ui/alert";
import { Card } from "@/ui/card";
import { Label } from "@/ui/label";
import { RadioGroup, RadioGroupItem } from "@/ui/radio-group";
import { Skeleton } from "@/ui/skeleton";

type Props = { settings: LoadedSettings; error: string | null; onChange: (s: Settings) => void };

const themes: [Theme, string][] = [
  ["system", "System"],
  ["light", "Light"],
  ["dark", "Dark"],
];

const closeOptions: [CloseBehavior, string, string][] = [
  ["tray", "Keep Flavor in the tray", "Closing the window hides it. Use the tray icon to show it again."],
  ["quit_gui", "Quit the window", "Closing the window quits the desktop app."],
];

export function SettingsPage({ settings, error, onChange }: Props) {
  const { info, instanceId, status } = useDaemon();
  const [version, setVersion] = useState("");
  const [keyring, setKeyring] = useState<string | null | undefined>(undefined);
  useEffect(() => {
    appVersion()
      .then(setVersion)
      .catch(() => setVersion("Unknown"));
  }, []);
  useEffect(() => {
    if (status !== "ready") return;
    runDiagnostics()
      .then((d) => setKeyring(keyringLabel(d.secretStoreBackend, d.secretStoreState)))
      .catch(() => setKeyring(null));
  }, [status]);

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Preferences for this desktop app. Each network has its own settings on the Networks page.
        </p>
      </header>

      {settings.warning && (
        <Alert role="status" className="text-warning">
          <TriangleAlert />
          <AlertDescription className="text-foreground">{settings.warning}</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert variant="destructive">
          <CircleAlert />
          <AlertTitle>Settings were not saved</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Section id="appearance" title="Appearance">
        <Card className="gap-4 p-4">
          <Heading id="theme" title="Theme" hint="System follows your operating system's light or dark setting." />
          <RadioGroup
            aria-labelledby="theme"
            value={settings.theme}
            onValueChange={(v) => onChange({ ...settings, theme: v as Theme })}
            className="flex flex-wrap gap-6"
          >
            {themes.map(([value, label]) => (
              <div key={value} className="flex items-center gap-2">
                <RadioGroupItem id={`theme-${value}`} value={value} />
                <Label htmlFor={`theme-${value}`} className="font-normal">
                  {label}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </Card>
      </Section>

      <Section id="behaviour" title="Behaviour">
        <Card className="gap-4 p-4">
          <Heading
            id="closing"
            title="When the window closes"
            hint="Networks stay connected either way. The Flavor daemon keeps running in the background."
          />
          <RadioGroup
            aria-labelledby="closing"
            value={settings.closeBehavior}
            onValueChange={(v) => onChange({ ...settings, closeBehavior: v as CloseBehavior })}
            className="gap-4"
          >
            {closeOptions.map(([value, label, hint]) => (
              <div key={value} className="flex items-start gap-3">
                <RadioGroupItem
                  id={`close-${value}`}
                  value={value}
                  aria-describedby={`close-${value}-hint`}
                  className="mt-0.5"
                />
                <div className="flex flex-col gap-1">
                  <Label htmlFor={`close-${value}`} className="font-normal">
                    {label}
                  </Label>
                  <p id={`close-${value}-hint`} className="text-xs text-muted-foreground">
                    {hint}
                  </p>
                </div>
              </div>
            ))}
          </RadioGroup>
        </Card>
      </Section>

      <Section id="about" title="About">
        <Card className="gap-0 py-0">
          <dl>
            <Fact term="Flavor desktop" mono>
              {version || <Skeleton className="h-4 w-20" />}
            </Fact>
            <Fact term="Daemon" mono>
              {info
                ? `${info.daemonVersion}${info.buildCommit ? ` (${info.buildCommit.slice(0, 12)})` : ""}`
                : "Not connected"}
            </Fact>
            <Fact term="Protocol" mono>
              {info ? `${info.protocolMajor}.${info.protocolMinor}` : "…"}
            </Fact>
            <Fact term="Keyring">{keyring === undefined && status === "ready" ? <Skeleton className="h-4 w-40" /> : (keyring ?? "Unknown")}</Fact>
          </dl>
          <details className="group border-t border-border px-4 py-3">
            <summary className="flex w-fit cursor-pointer list-none items-center gap-1 rounded-md text-sm text-muted-foreground outline-none select-none hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 [&::-webkit-details-marker]:hidden">
              <ChevronRight className="size-4 transition-transform group-open:rotate-90" aria-hidden="true" />
              Daemon instance
            </summary>
            <p className="mt-2 pl-5 font-mono text-xs break-all">{instanceId || "…"}</p>
          </details>
        </Card>
      </Section>
    </div>
  );
}

function Section({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <section aria-labelledby={id} className="flex flex-col gap-2">
      <h2 id={id} className="text-sm font-medium text-muted-foreground">
        {title}
      </h2>
      {children}
    </section>
  );
}

function Heading({ id, title, hint }: { id: string; title: string; hint: string }) {
  return (
    <div className="flex flex-col gap-1">
      <h3 id={id} className="text-sm font-medium">
        {title}
      </h3>
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  );
}

function Fact({ term, mono, children }: { term: string; mono?: boolean; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-t border-border px-4 py-3 first:border-t-0">
      <dt className="text-sm text-muted-foreground">{term}</dt>
      <dd className={mono ? "text-right font-mono text-sm" : "text-right text-sm"}>{children}</dd>
    </div>
  );
}
