import { useEffect, useMemo, useRef, useState } from "react";
import { toJsonString } from "@bufbuild/protobuf";
import {
  CircleAlert,
  CircleCheck,
  CircleDashed,
  CircleX,
  Copy,
  type LucideIcon,
  RefreshCw,
  TriangleAlert,
  Unplug,
} from "lucide-react";
import { motion } from "motion/react";
import { toast } from "sonner";
import { RunDiagnosticsResponseSchema, type RunDiagnosticsResponse } from "@gen/flavor/v1/diagnostics_pb";
import { runDiagnostics } from "../../lib/api/daemon";
import { useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Alert, AlertDescription, AlertTitle } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Card } from "@/ui/card";
import { Skeleton } from "@/ui/skeleton";
import {
  checkLabel,
  checkTone,
  detailParts,
  isNetworkCheck,
  statusLabel,
  systemChecks,
  type CheckRow,
  type Tone,
} from "./checks";

const toneStyle: Record<Tone, [LucideIcon, string]> = {
  ok: [CircleCheck, "text-success"],
  warn: [TriangleAlert, "text-warning"],
  error: [CircleX, "text-destructive"],
  unknown: [CircleDashed, "text-muted-foreground"],
};

export function DiagnosticsPage() {
  const { networks, status } = useDaemon();
  const [result, setResult] = useState<{ response: RunDiagnosticsResponse; at: Date } | null>(null);
  const run = useAction(runDiagnostics);
  const heading = useRef<HTMLHeadingElement>(null);
  const names = useMemo(() => new Map([...networks.values()].map((n) => [n.id, n.displayName])), [networks]);
  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);

  const refresh = async () => {
    if (run.pending) return;
    const r = await run.run();
    if (r.ok) setResult({ response: r.value, at: new Date() });
  };
  useEffect(() => {
    if (status === "ready") void refresh();
  }, [status]);

  const copyReport = () => {
    if (!result) return;
    navigator.clipboard
      .writeText(toJsonString(RunDiagnosticsResponseSchema, result.response, { prettySpaces: 2 }))
      .then(
        () => toast.success("Report copied"),
        () => toast.error("Could not copy the report"),
      );
  };

  const networkChecks = result?.response.checks.filter((c) => isNetworkCheck(c.name)) ?? [];

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="flex items-start justify-between gap-4">
        <div className="flex min-w-0 flex-col gap-1">
          <h1 ref={heading} tabIndex={-1} className="text-xl font-semibold outline-none">
            Diagnostics
          </h1>
          <p className="text-sm text-muted-foreground">
            A safe summary of the daemon and each network, without keys or sign-in links.
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button variant="outline" size="sm" disabled={!result} onClick={copyReport}>
            <Copy />
            Copy report
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={status !== "ready"}
            aria-busy={run.pending || undefined}
            onClick={() => void refresh()}
          >
            <RefreshCw className={cn(run.pending && "animate-spin")} />
            Run again
          </Button>
        </div>
      </header>
      <p role="status" className="sr-only">
        {run.pending ? "Running checks" : result ? `${result.response.checks.length} checks finished` : ""}
      </p>

      {run.error && (
        <Alert variant="destructive">
          <CircleAlert />
          <AlertTitle>Diagnostics did not run</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>{run.error}</span>
            <Button
              size="sm"
              variant="outline"
              className="text-foreground"
              disabled={status !== "ready"}
              onClick={() => {
                heading.current?.focus();
                void refresh();
              }}
            >
              Try again
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {result ? (
        <>
          <CheckSection
            title="System"
            meta={`Last run ${result.at.toLocaleTimeString()}`}
            rows={systemChecks(result.response)}
            names={names}
          />
          {networkChecks.length > 0 && (
            <CheckSection title="Networks" rows={networkChecks} names={names} colors={colors} />
          )}
        </>
      ) : run.error ? null : run.pending || status === "ready" ? (
        <ChecksSkeleton />
      ) : (
        <Alert role="status">
          <Unplug />
          <AlertTitle>Not connected to the Flavor daemon</AlertTitle>
          <AlertDescription>Diagnostics run again when it reconnects.</AlertDescription>
        </Alert>
      )}
    </div>
  );
}

function CheckSection({
  title,
  meta,
  rows,
  names,
  colors,
}: {
  title: string;
  meta?: string;
  rows: CheckRow[];
  names: Map<string, string>;
  colors?: Map<string, number>;
}) {
  const id = `checks-${title.toLowerCase()}`;
  return (
    <section aria-labelledby={id} className="flex flex-col gap-2">
      <div className="flex items-baseline justify-between gap-4">
        <h2 id={id} className="text-sm font-medium text-muted-foreground">
          {title}
        </h2>
        {meta && <span className="text-xs text-muted-foreground tabular-nums">{meta}</span>}
      </div>
      <Card className="gap-0 py-0">
        <ul>
          {rows.map((c, i) => {
            const net = c.name.slice(8);
            return (
              <CheckItem
                key={c.name}
                check={c}
                index={i}
                label={checkLabel(c.name, names)}
                color={colors?.has(net) ? networkColorVar(colors.get(net)) : undefined}
              />
            );
          })}
        </ul>
      </Card>
    </section>
  );
}

function CheckItem({ check, index, label, color }: { check: CheckRow; index: number; label: string; color?: string }) {
  const [Icon, toneClass] = toneStyle[checkTone(check.status)];
  const { facts, reports } = detailParts(check.safeDetail);
  return (
    <motion.li
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: index * 0.04, duration: 0.18 }}
      className="flex items-start gap-3 border-t border-border px-4 py-3 first:border-t-0"
    >
      <Icon className={cn("mt-0.5 size-4 shrink-0", toneClass)} aria-hidden="true" />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex items-center gap-2">
          {color && (
            <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: color }} aria-hidden="true" />
          )}
          <span className="truncate text-sm font-medium">{label}</span>
          <span className={cn("ml-auto shrink-0 text-xs font-medium", toneClass)}>
            {statusLabel(check.status)}
          </span>
        </div>
        {facts && <p className="text-xs break-words text-muted-foreground">{facts}</p>}
        {reports.length > 0 && (
          <div className="mt-1 flex flex-col gap-1 rounded-md bg-muted px-3 py-2 text-xs">
            <p className="font-medium">Control server reports</p>
            <ul className="flex list-disc flex-col gap-1 pl-4 text-muted-foreground">
              {reports.map((r, i) => (
                <li key={i}>{r}</li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </motion.li>
  );
}

function ChecksSkeleton() {
  return (
    <section aria-busy="true" aria-label="Running diagnostics" className="flex flex-col gap-2">
      <Skeleton className="h-4 w-16" />
      <Card className="gap-0 py-0">
        {[0, 1, 2].map((i) => (
          <div key={i} className="flex items-start gap-3 border-t border-border px-4 py-3 first:border-t-0">
            <Skeleton className="size-4 rounded-full" />
            <div className="flex flex-1 flex-col gap-2">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-3 w-64 max-w-full" />
            </div>
          </div>
        ))}
      </Card>
    </section>
  );
}
