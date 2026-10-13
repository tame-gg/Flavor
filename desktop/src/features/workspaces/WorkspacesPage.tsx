import { useId, useMemo, useState, type ReactNode } from "react";
import { Boxes, ChevronRight, Loader2, Pencil, Plus, Trash2 } from "lucide-react";
import { motion } from "motion/react";
import { toast } from "sonner";
import { NetworkConnectionState as S } from "@gen/flavor/v1/common_pb";
import type { Network } from "@gen/flavor/v1/network_pb";
import { ActivationOutcome, type ActivateWorkspaceResponse, type Workspace } from "@gen/flavor/v1/workspaces_pb";
import { stateLabel } from "@/lib/networkState";
import {
  activateWorkspace,
  createWorkspace,
  deactivateWorkspace,
  deleteWorkspace,
  updateWorkspace,
} from "../../lib/api/daemon";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { providerName } from "../networks/format";
import type { Tone } from "../overview/summary";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Badge } from "@/ui/badge";
import { Button } from "@/ui/button";
import { Card } from "@/ui/card";
import { Checkbox } from "@/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/ui/dialog";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { activationPlan, memberSummary } from "./plan";

type Colors = Map<string, number>;

const toneDot: Record<Tone, string> = {
  ok: "bg-success",
  warn: "bg-warning",
  danger: "bg-destructive",
  neutral: "bg-muted-foreground/60",
};

const stateTone: Partial<Record<S, Tone>> = {
  [S.CONNECTED]: "ok",
  [S.DEGRADED]: "warn",
  [S.AUTHENTICATING]: "warn",
  [S.AWAITING_APPROVAL]: "warn",
  [S.ERROR]: "danger",
};

const outcomes: Record<ActivationOutcome, [string, Tone]> = {
  [ActivationOutcome.UNSPECIFIED]: ["", "neutral"],
  [ActivationOutcome.CONNECTING]: ["Connecting", "ok"],
  [ActivationOutcome.ALREADY_ACTIVE]: ["Already connected", "ok"],
  [ActivationOutcome.DISCONNECTED]: ["Disconnected", "neutral"],
  [ActivationOutcome.FAILED]: ["Failed", "danger"],
};

export function WorkspacesPage({ onOpenNetwork }: { onOpenNetwork: (id: string) => void }) {
  const { workspaces, networks, activeWorkspaceId } = useDaemon();
  const canMutate = useCanMutate();
  const [editing, setEditing] = useState<Workspace | "new" | null>(null);
  const [activating, setActivating] = useState<Workspace | null>(null);
  const [deleting, setDeleting] = useState<Workspace | null>(null);
  const deactivation = useAction(deactivateWorkspace);
  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);
  const list = [...workspaces.values()].sort(
    (a, b) => Number(b.id === activeWorkspaceId) - Number(a.id === activeWorkspaceId) || a.name.localeCompare(b.name),
  );

  const deactivate = async (ws: Workspace) => {
    const r = await deactivation.run();
    if (r.ok) toast.success(`Deactivated ${ws.name}`);
    else toast.error(`Couldn't deactivate ${ws.name}`, { description: r.error });
  };

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-xl font-semibold">Workspaces</h1>
          <p className="text-sm text-muted-foreground">Group networks by task and connect them in one step.</p>
        </div>
        <Button disabled={!canMutate} onClick={() => setEditing("new")}>
          <Plus /> New workspace
        </Button>
      </header>

      {list.length === 0 ? (
        <Card className="items-center gap-4 px-6 py-10 text-center">
          <Boxes className="size-6 text-muted-foreground" aria-hidden="true" />
          <div className="flex flex-col gap-1">
            <h2 className="text-sm font-medium">No workspaces yet</h2>
            <p className="max-w-md text-sm text-balance text-muted-foreground">
              A workspace is a named set of networks, like Work, Home or On call. Activate one to connect everything you
              need for that context in one step.
            </p>
          </div>
          <Button disabled={!canMutate || networks.size === 0} onClick={() => setEditing("new")}>
            Create a workspace
          </Button>
          {networks.size === 0 && <p className="text-xs text-muted-foreground">Add a network first.</p>}
        </Card>
      ) : (
        <ul className="flex flex-col gap-3">
          {list.map((ws, i) => {
            const active = ws.id === activeWorkspaceId;
            const members = ws.networkIds.map((id) => networks.get(id)).filter((n): n is Network => n !== undefined);
            return (
              <motion.li
                key={ws.id}
                initial={{ opacity: 0, y: 4 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: i * 0.04, duration: 0.18 }}
              >
                <Card
                  className={cn("gap-0 overflow-hidden py-0", active && "border-primary/60 ring-1 ring-primary/20")}
                >
                  <div className="flex flex-wrap items-start gap-3 px-4 py-3">
                    <div className="flex min-w-0 flex-1 flex-col gap-1">
                      <div className="flex items-center gap-2">
                        <h2 className="truncate text-sm font-semibold">{ws.name}</h2>
                        {active && <Badge>Active</Badge>}
                      </div>
                      {ws.description && <p className="text-sm text-muted-foreground">{ws.description}</p>}
                      {members.length > 0 && (
                        <p className="text-xs text-muted-foreground">{memberSummary(ws, networks)}</p>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      {active && (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!canMutate || deactivation.pending}
                          onClick={() => void deactivate(ws)}
                        >
                          {deactivation.pending && <Loader2 className="animate-spin" />}
                          Deactivate
                        </Button>
                      )}
                      <Button size="sm" variant="outline" disabled={!canMutate} onClick={() => setActivating(ws)}>
                        {active ? "Activate again" : "Activate"}
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        disabled={!canMutate}
                        aria-label={`Edit ${ws.name}`}
                        title="Edit"
                        onClick={() => setEditing(ws)}
                      >
                        <Pencil />
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        disabled={!canMutate}
                        aria-label={`Delete ${ws.name}`}
                        title="Delete"
                        className="hover:text-destructive"
                        onClick={() => setDeleting(ws)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </div>
                  {members.length > 0 ? (
                    <ul aria-label={`Networks in ${ws.name}`}>
                      {members.map((n) => (
                        <li key={n.id} className="border-t border-border">
                          <button
                            type="button"
                            onClick={() => onOpenNetwork(n.id)}
                            className="group flex w-full items-center gap-3 px-4 py-2 text-left transition-colors hover:bg-accent/50 focus-visible:bg-accent/50 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none focus-visible:ring-inset"
                          >
                            <Swatch color={networkColorVar(colors.get(n.id))} />
                            <span className="min-w-0 flex-1 truncate text-sm">{n.displayName}</span>
                            <ToneLabel tone={stateTone[n.state] ?? "neutral"}>{stateLabel(n.state)}</ToneLabel>
                            <ChevronRight
                              className="size-4 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100"
                              aria-hidden="true"
                            />
                          </button>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="border-t border-border px-4 py-3 text-xs text-muted-foreground">
                      This workspace has no networks. Edit it to add some.
                    </p>
                  )}
                </Card>
              </motion.li>
            );
          })}
        </ul>
      )}

      {editing && (
        <EditorDialog
          key={editing === "new" ? "new" : editing.id}
          workspace={editing}
          colors={colors}
          onClose={() => setEditing(null)}
        />
      )}
      {activating && (
        <ActivateDialog
          key={activating.id}
          workspace={activating}
          colors={colors}
          onClose={() => setActivating(null)}
        />
      )}
      {deleting && <DeleteDialog key={deleting.id} workspace={deleting} onClose={() => setDeleting(null)} />}
    </div>
  );
}

function Swatch({ color }: { color: string }) {
  return <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: color }} aria-hidden="true" />;
}

function ToneLabel({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
      <span className={cn("size-1.5 rounded-full", toneDot[tone])} aria-hidden="true" />
      {children}
    </span>
  );
}

function PageDialog({ busy, onClose, children }: { busy: boolean; onClose: () => void; children: ReactNode }) {
  const [opener] = useState(() => document.activeElement as HTMLElement | null);
  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent
        className="sm:max-w-md"
        onCloseAutoFocus={(e) => {
          e.preventDefault();
          opener?.focus();
        }}
      >
        {children}
      </DialogContent>
    </Dialog>
  );
}

function ErrorText({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="text-sm text-destructive">
      {children}
    </p>
  );
}

function EditorDialog({
  workspace,
  colors,
  onClose,
}: {
  workspace: Workspace | "new";
  colors: Colors;
  onClose: () => void;
}) {
  const { networks } = useDaemon();
  const canMutate = useCanMutate();
  const ids = useId();
  const existing = workspace === "new" ? null : workspace;
  const [name, setName] = useState(existing?.name ?? "");
  const [description, setDescription] = useState(existing?.description ?? "");
  const [selected, setSelected] = useState<Set<string>>(new Set(existing?.networkIds ?? []));
  const save = useAction(async () => {
    const networkIds = [...selected].filter((id) => networks.has(id));
    if (existing)
      return updateWorkspace(existing.id, { name: name.trim(), description: description.trim(), networkIds });
    return createWorkspace({ name: name.trim(), description: description.trim(), networkIds });
  });
  const sorted = [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName));
  const valid = name.trim() !== "";
  const submit = async () => {
    const r = await save.run();
    if (!r.ok) return;
    toast.success(existing ? `Saved ${r.value.name}` : `Created ${r.value.name}`);
    onClose();
  };
  const toggle = (id: string, on: boolean) => {
    const next = new Set(selected);
    if (on) next.add(id);
    else next.delete(id);
    setSelected(next);
  };

  return (
    <PageDialog busy={save.pending} onClose={onClose}>
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (valid && canMutate && !save.pending) void submit();
        }}
      >
        <DialogHeader>
          <DialogTitle>{existing ? `Edit ${existing.name}` : "New workspace"}</DialogTitle>
          <DialogDescription>Activating the workspace connects the networks you pick here.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          <Label htmlFor={`${ids}-name`}>Name</Label>
          <Input
            id={`${ids}-name`}
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={64}
            placeholder="On call"
            required
            autoFocus
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor={`${ids}-description`}>
            Description <span className="font-normal text-muted-foreground">(optional)</span>
          </Label>
          <Input
            id={`${ids}-description`}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            maxLength={256}
            placeholder="Production and monitoring for pager duty"
          />
        </div>
        <div role="group" aria-labelledby={`${ids}-networks`} className="flex flex-col gap-2">
          <span id={`${ids}-networks`} className="text-sm font-medium">
            Networks
          </span>
          {sorted.length === 0 ? (
            <p className="text-sm text-muted-foreground">Add a network first.</p>
          ) : (
            <div className="max-h-64 overflow-y-auto rounded-md border border-border">
              {sorted.map((n) => (
                <label
                  key={n.id}
                  className="flex cursor-pointer items-center gap-3 border-t border-border px-3 py-2 transition-colors first:border-t-0 hover:bg-accent/50"
                >
                  <Checkbox checked={selected.has(n.id)} onCheckedChange={(v) => toggle(n.id, v === true)} />
                  <Swatch color={networkColorVar(colors.get(n.id))} />
                  <span className="min-w-0 flex-1 truncate text-sm">{n.displayName}</span>
                  <span className="text-xs text-muted-foreground">{providerName(n.provider)}</span>
                </label>
              ))}
            </div>
          )}
        </div>
        {save.error && <ErrorText>{save.error}</ErrorText>}
        <DialogFooter>
          <Button type="button" variant="outline" disabled={save.pending} onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!valid || !canMutate || save.pending}>
            {save.pending && <Loader2 className="animate-spin" />}
            {existing ? "Save" : "Create workspace"}
          </Button>
        </DialogFooter>
      </form>
    </PageDialog>
  );
}

function ActivateDialog({ workspace, colors, onClose }: { workspace: Workspace; colors: Colors; onClose: () => void }) {
  const { networks } = useDaemon();
  const canMutate = useCanMutate();
  const ids = useId();
  const [disconnectOthers, setDisconnectOthers] = useState(false);
  const [result, setResult] = useState<ActivateWorkspaceResponse | null>(null);
  const activate = useAction(activateWorkspace);
  const plan = activationPlan(workspace, networks);
  const names = (ns: Network[]) => ns.map((n) => n.displayName).join(", ");
  const submit = async () => {
    if (result) return onClose();
    const r = await activate.run(workspace.id, disconnectOthers);
    if (r.ok) setResult(r.value);
  };

  return (
    <PageDialog busy={activate.pending} onClose={onClose}>
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <p role="status" className="sr-only">
          {activate.pending
            ? `Activating ${workspace.name}`
            : result
              ? `${workspace.name} is active. ${result.results.filter((r) => r.outcome === ActivationOutcome.FAILED).length} of ${result.results.length} networks failed.`
              : ""}
        </p>
        <DialogHeader>
          <DialogTitle>{result ? `${workspace.name} is active` : `Activate ${workspace.name}`}</DialogTitle>
          <DialogDescription>
            {result
              ? "What happened to each network."
              : "Activation connects and disconnects networks only. Flavor does not change how traffic is routed between them."}
          </DialogDescription>
        </DialogHeader>
        {result ? (
          result.results.length === 0 ? (
            <p className="text-sm text-muted-foreground">Nothing needed to change.</p>
          ) : (
            <ul className="rounded-md border border-border">
              {result.results.map((r, i) => {
                const [label, tone] = outcomes[r.outcome] ?? outcomes[ActivationOutcome.UNSPECIFIED];
                return (
                  <li
                    key={r.network?.id ?? i}
                    className="flex flex-col gap-1 border-t border-border px-3 py-2 first:border-t-0"
                  >
                    <div className="flex items-center gap-3">
                      <Swatch color={networkColorVar(colors.get(r.network?.id ?? ""))} />
                      <span className="min-w-0 flex-1 truncate text-sm font-medium">{r.network?.displayName}</span>
                      {label && <ToneLabel tone={tone}>{label}</ToneLabel>}
                    </div>
                    {r.safeMessage && <p className="pl-5.5 text-xs text-muted-foreground">{r.safeMessage}</p>}
                  </li>
                );
              })}
            </ul>
          )
        ) : (
          <>
            {plan.connect.length + plan.alreadyUp.length === 0 ? (
              <p className="text-sm text-muted-foreground">This workspace has no networks to connect.</p>
            ) : (
              <div className="flex flex-col gap-3">
                <PlanGroup label="Will connect" networks={plan.connect} colors={colors} />
                <PlanGroup label="Already started" networks={plan.alreadyUp} colors={colors} />
              </div>
            )}
            {plan.outside.length > 0 && (
              <div className="flex items-start gap-3 rounded-md border border-border p-3">
                <Checkbox
                  id={`${ids}-others`}
                  checked={disconnectOthers}
                  onCheckedChange={(v) => setDisconnectOthers(v === true)}
                  aria-describedby={`${ids}-others-hint`}
                  className="mt-0.5"
                />
                <div className="flex flex-col gap-1">
                  <Label htmlFor={`${ids}-others`} className="leading-snug">
                    Also disconnect {names(plan.outside)}
                  </Label>
                  <p id={`${ids}-others-hint`} className="text-xs text-muted-foreground">
                    {plan.outside.length === 1 ? "Its identity is kept" : "Their identities are kept"}; you can
                    reconnect at any time.
                  </p>
                </div>
              </div>
            )}
          </>
        )}
        {activate.error && <ErrorText>{activate.error}</ErrorText>}
        <DialogFooter>
          {result ? (
            <Button type="submit" autoFocus>
              Done
            </Button>
          ) : (
            <>
              <Button type="button" variant="outline" disabled={activate.pending} onClick={onClose}>
                Cancel
              </Button>
              <Button type="submit" disabled={!canMutate || activate.pending}>
                {activate.pending && <Loader2 className="animate-spin" />}
                Activate
              </Button>
            </>
          )}
        </DialogFooter>
      </form>
    </PageDialog>
  );
}

function PlanGroup({ label, networks, colors }: { label: string; networks: Network[]; colors: Colors }) {
  if (networks.length === 0) return null;
  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-xs font-medium text-muted-foreground">{label}</h3>
      <ul className="flex flex-wrap gap-2">
        {networks.map((n) => (
          <li key={n.id} className="flex items-center gap-2 rounded-md border border-border px-2 py-1 text-sm">
            <Swatch color={networkColorVar(colors.get(n.id))} />
            {n.displayName}
          </li>
        ))}
      </ul>
    </div>
  );
}

function DeleteDialog({ workspace, onClose }: { workspace: Workspace; onClose: () => void }) {
  const canMutate = useCanMutate();
  const remove = useAction(deleteWorkspace);
  const confirm = async () => {
    if (!(await remove.run(workspace.id)).ok) return;
    toast.success(`Deleted ${workspace.name}`);
    onClose();
  };
  return (
    <PageDialog busy={remove.pending} onClose={onClose}>
      <DialogHeader>
        <DialogTitle>Delete {workspace.name}?</DialogTitle>
        <DialogDescription>
          The workspace is removed. Its networks, their connections and their identities are not affected.
        </DialogDescription>
      </DialogHeader>
      {remove.error && <ErrorText>{remove.error}</ErrorText>}
      <DialogFooter>
        <Button variant="outline" disabled={remove.pending} onClick={onClose}>
          Cancel
        </Button>
        <Button variant="destructive" disabled={!canMutate || remove.pending} onClick={() => void confirm()}>
          {remove.pending && <Loader2 className="animate-spin" />}
          Delete workspace
        </Button>
      </DialogFooter>
    </PageDialog>
  );
}
