import { useState } from "react";
import type { Network } from "@gen/lattice/v1/network_pb";
import { ActivationOutcome, type ActivateWorkspaceResponse, type Workspace } from "@gen/lattice/v1/workspaces_pb";
import { Button } from "../../components/ui/Button";
import { Dialog } from "../../components/ui/Dialog";
import { Field } from "../../components/ui/Field";
import { StatusBadge } from "../../components/ui/StatusBadge";
import { activateWorkspace, createWorkspace, deactivateWorkspace, deleteWorkspace, updateWorkspace } from "../../lib/api/daemon";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { providerName } from "../networks/format";
import { activationPlan, memberSummary } from "./plan";

const outcomeLabel: Record<ActivationOutcome, string> = {
  [ActivationOutcome.UNSPECIFIED]: "",
  [ActivationOutcome.CONNECTING]: "Connecting",
  [ActivationOutcome.ALREADY_ACTIVE]: "Already connected",
  [ActivationOutcome.DISCONNECTED]: "Disconnected",
  [ActivationOutcome.FAILED]: "Failed",
};

export function WorkspacesPage({ onOpenNetwork }: { onOpenNetwork: (id: string) => void }) {
  const { workspaces, networks, activeWorkspaceId } = useDaemon();
  const canMutate = useCanMutate();
  const [editing, setEditing] = useState<Workspace | "new" | null>(null);
  const [activating, setActivating] = useState<Workspace | null>(null);
  const [deleting, setDeleting] = useState<Workspace | null>(null);
  const deactivate = useAction(deactivateWorkspace);
  const list = [...workspaces.values()].sort(
    (a, b) => Number(b.id === activeWorkspaceId) - Number(a.id === activeWorkspaceId) || a.name.localeCompare(b.name),
  );

  return (
    <div className="page">
      <header className="page-header">
        <div className="stack-sm">
          <h1>Workspaces</h1>
          <p className="muted">Group networks around what you are doing. Activating a workspace connects its networks.</p>
        </div>
        <Button variant="primary" disabled={!canMutate} onClick={() => setEditing("new")}>
          New workspace
        </Button>
      </header>

      {deactivate.error && <p className="error-text">{deactivate.error}</p>}

      {list.length === 0 ? (
        <div className="card empty">
          <h2>No workspaces yet</h2>
          <p className="muted">
            A workspace is a named set of networks, like Work, Home or On Call. Activate one to connect everything you
            need for that context in one step.
          </p>
          <Button variant="primary" disabled={!canMutate || networks.size === 0} onClick={() => setEditing("new")}>
            Create a workspace
          </Button>
        </div>
      ) : (
        <ul className="list candidate-list">
          {list.map((ws) => {
            const active = ws.id === activeWorkspaceId;
            const members = ws.networkIds.map((id) => networks.get(id)).filter((n): n is Network => n !== undefined);
            return (
              <li key={ws.id} className={`card candidate ${active ? "workspace-active" : ""}`}>
                <div className="row">
                  <strong className="network-name">{ws.name}</strong>
                  {active && <span className="badge badge-info">Active</span>}
                  <span className="muted small">{memberSummary(ws, networks)}</span>
                  <span className="spacer" />
                  {active ? (
                    <Button loading={deactivate.pending} disabled={!canMutate} onClick={() => void deactivate.run()}>
                      Deactivate
                    </Button>
                  ) : null}
                  <Button variant={active ? "secondary" : "primary"} disabled={!canMutate} onClick={() => setActivating(ws)}>
                    {active ? "Activate again" : "Activate"}
                  </Button>
                </div>
                {ws.description && <p className="muted">{ws.description}</p>}
                {members.length > 0 ? (
                  <div className="row">
                    {members.map((n) => (
                      <button key={n.id} type="button" className="member-chip" onClick={() => onOpenNetwork(n.id)}>
                        <span>{n.displayName}</span>
                        <StatusBadge state={n.state} />
                      </button>
                    ))}
                  </div>
                ) : (
                  <p className="muted small">This workspace has no networks. Edit it to add some.</p>
                )}
                <div className="row">
                  <span className="spacer" />
                  <Button variant="ghost" disabled={!canMutate} onClick={() => setEditing(ws)}>
                    Edit
                  </Button>
                  <Button variant="danger" disabled={!canMutate} onClick={() => setDeleting(ws)}>
                    Delete…
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      <WorkspaceEditor workspace={editing} onClose={() => setEditing(null)} />
      <ActivateDialog workspace={activating} onClose={() => setActivating(null)} />
      <DeleteDialog workspace={deleting} onClose={() => setDeleting(null)} />
    </div>
  );
}

function WorkspaceEditor({ workspace, onClose }: { workspace: Workspace | "new" | null; onClose: () => void }) {
  const open = workspace !== null;
  return open ? <EditorForm key={workspace === "new" ? "new" : workspace.id} workspace={workspace} onClose={onClose} /> : null;
}

function EditorForm({ workspace, onClose }: { workspace: Workspace | "new"; onClose: () => void }) {
  const { networks } = useDaemon();
  const canMutate = useCanMutate();
  const existing = workspace === "new" ? null : workspace;
  const [name, setName] = useState(existing?.name ?? "");
  const [description, setDescription] = useState(existing?.description ?? "");
  const [selected, setSelected] = useState<Set<string>>(new Set(existing?.networkIds ?? []));
  const save = useAction(async () => {
    const networkIds = [...selected].filter((id) => networks.has(id));
    if (existing) return updateWorkspace(existing.id, { name: name.trim(), description: description.trim(), networkIds });
    return createWorkspace({ name: name.trim(), description: description.trim(), networkIds });
  });
  const sorted = [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName));
  const valid = name.trim() !== "";
  const submit = async () => {
    if ((await save.run()).ok) onClose();
  };
  return (
    <Dialog
      open
      title={existing ? `Edit ${existing.name}` : "New workspace"}
      onClose={onClose}
      onSubmit={() => valid && canMutate && void submit()}
      actions={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={save.pending} disabled={!valid || !canMutate}>
            {existing ? "Save" : "Create workspace"}
          </Button>
        </>
      }
    >
      <Field label="Name" value={name} onChange={(e) => setName(e.target.value)} maxLength={64} placeholder="On Call" required autoFocus />
      <Field
        label="Description (optional)"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        maxLength={256}
        placeholder="Production and monitoring for pager duty"
      />
      <fieldset className="field network-picker">
        <legend className="field-label">Networks</legend>
        {sorted.length === 0 && <p className="muted small">Add a network first.</p>}
        {sorted.map((n) => (
          <label key={n.id} className="check">
            <input
              type="checkbox"
              checked={selected.has(n.id)}
              onChange={(e) => {
                const next = new Set(selected);
                if (e.target.checked) next.add(n.id);
                else next.delete(n.id);
                setSelected(next);
              }}
            />
            <span className="grow">{n.displayName}</span>
            <span className="muted small">{providerName(n.provider)}</span>
          </label>
        ))}
      </fieldset>
      {save.error && <p className="error-text" role="alert">{save.error}</p>}
    </Dialog>
  );
}

function ActivateDialog({ workspace, onClose }: { workspace: Workspace | null; onClose: () => void }) {
  const { networks } = useDaemon();
  const canMutate = useCanMutate();
  const [disconnectOthers, setDisconnectOthers] = useState(false);
  const [result, setResult] = useState<ActivateWorkspaceResponse | null>(null);
  const activate = useAction(activateWorkspace);
  const close = () => {
    setResult(null);
    setDisconnectOthers(false);
    activate.clearError();
    onClose();
  };
  const plan = workspace ? activationPlan(workspace, networks) : null;
  const names = (ns: Network[]) => ns.map((n) => n.displayName).join(", ");

  return (
    <Dialog
      open={workspace !== null}
      title={result ? `${workspace?.name} is active` : `Activate ${workspace?.name ?? ""}`}
      onClose={close}
      onSubmit={async () => {
        if (!workspace || result) return close();
        const r = await activate.run(workspace.id, disconnectOthers);
        if (r.ok) setResult(r.value);
      }}
      actions={
        result ? (
          <Button type="submit" variant="primary">
            Done
          </Button>
        ) : (
          <>
            <Button onClick={close}>Cancel</Button>
            <Button type="submit" variant="primary" loading={activate.pending} disabled={!canMutate}>
              Activate
            </Button>
          </>
        )
      }
    >
      {result ? (
        result.results.length === 0 ? (
          <p className="muted">Nothing needed to change.</p>
        ) : (
          <ul className="list">
            {result.results.map((r) => (
              <li key={r.network?.id} className="row">
                <strong className="grow">{r.network?.displayName}</strong>
                <span className={`badge ${r.outcome === ActivationOutcome.FAILED ? "badge-danger" : r.outcome === ActivationOutcome.DISCONNECTED ? "" : "badge-ok"}`}>
                  {outcomeLabel[r.outcome]}
                </span>
                {r.safeMessage && <span className="muted small">{r.safeMessage}</span>}
              </li>
            ))}
          </ul>
        )
      ) : (
        plan && (
          <>
            <dl className="dl">
              {plan.connect.length > 0 && (
                <>
                  <dt>Will connect</dt>
                  <dd>{names(plan.connect)}</dd>
                </>
              )}
              {plan.alreadyUp.length > 0 && (
                <>
                  <dt>Already started</dt>
                  <dd>{names(plan.alreadyUp)}</dd>
                </>
              )}
            </dl>
            {plan.outside.length > 0 && (
              <label className="check">
                <input type="checkbox" checked={disconnectOthers} onChange={(e) => setDisconnectOthers(e.target.checked)} />
                <span>
                  Also disconnect {names(plan.outside)}
                  <span className="hint check-hint">
                    {plan.outside.length === 1 ? "Its identity is kept" : "Their identities are kept"}; you can reconnect at any time.
                  </span>
                </span>
              </label>
            )}
            <p className="hint">
              Activation connects and disconnects networks only. Lattice does not change how traffic is routed between them.
            </p>
          </>
        )
      )}
      {activate.error && <p className="error-text" role="alert">{activate.error}</p>}
    </Dialog>
  );
}

function DeleteDialog({ workspace, onClose }: { workspace: Workspace | null; onClose: () => void }) {
  const canMutate = useCanMutate();
  const remove = useAction(deleteWorkspace);
  return (
    <Dialog
      open={workspace !== null}
      title={`Delete ${workspace?.name ?? "workspace"}?`}
      onClose={onClose}
      onSubmit={async () => {
        if (workspace && (await remove.run(workspace.id)).ok) onClose();
      }}
      actions={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="danger-solid" loading={remove.pending} disabled={!canMutate}>
            Delete workspace
          </Button>
        </>
      }
    >
      <p>The workspace is removed. Its networks, their connections and their identities are not affected.</p>
      {remove.error && <p className="error-text" role="alert">{remove.error}</p>}
    </Dialog>
  );
}
