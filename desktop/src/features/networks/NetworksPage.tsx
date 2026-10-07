import { useMemo, useState } from "react";
import type { Network } from "@gen/lattice/v1/network_pb";
import { Button } from "../../components/ui/Button";
import { StatusBadge } from "../../components/ui/StatusBadge";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { AddNetworkDialog } from "./AddNetworkDialog";
import { NetworkDetail } from "./NetworkDetail";
import { RemoveNetworkDialog } from "./RemoveNetworkDialog";
import { controlHost, providerName } from "./format";

type Props = {
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onShowDevices: (networkId: string) => void;
  addOpen: boolean;
  setAddOpen: (open: boolean) => void;
};

export function NetworksPage({ selectedId, onSelect, onShowDevices, addOpen, setAddOpen }: Props) {
  const { networks, devices } = useDaemon();
  const canMutate = useCanMutate();
  const [removing, setRemoving] = useState<Network | null>(null);
  const [notice, setNotice] = useState<{ id: string; text: string } | null>(null);

  const list = useMemo(
    () => [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName) || a.id.localeCompare(b.id)),
    [networks],
  );
  const counts = useMemo(() => {
    const m = new Map<string, number>();
    for (const d of devices.values()) {
      const id = d.id?.networkId ?? "";
      m.set(id, (m.get(id) ?? 0) + 1);
    }
    return m;
  }, [devices]);
  const selected = (selectedId && networks.get(selectedId)) || list[0] || null;

  return (
    <div className="page">
      <header className="page-header">
        <div className="stack-sm">
          <h1>Networks</h1>
          <p className="muted">Each network runs its own isolated session with its own device identity.</p>
        </div>
        <Button variant="primary" disabled={!canMutate} onClick={() => setAddOpen(true)}>
          Add network
        </Button>
      </header>

      {list.length === 0 ? (
        <div className="card empty">
          <h2>No networks yet</h2>
          <p className="muted">
            Add a Tailscale account or a Headscale server. You can join several at once; each keeps its own devices and
            addresses.
          </p>
          <Button variant="primary" disabled={!canMutate} onClick={() => setAddOpen(true)}>
            Add your first network
          </Button>
        </div>
      ) : (
        <div className="split">
          <nav aria-label="Networks">
            <ul className="list">
              {list.map((n) => (
                <li key={n.id}>
                  <button
                    type="button"
                    className="list-item"
                    aria-current={selected?.id === n.id}
                    onClick={() => onSelect(n.id)}
                  >
                    <span className="row list-head">
                      <span className="list-title">{n.displayName}</span>
                      <StatusBadge state={n.state} />
                    </span>
                    <span className="cell small muted">
                      {n.controlUrl ? `${providerName(n.provider)} · ${controlHost(n)}` : providerName(n.provider)}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </nav>
          {selected && (
            <NetworkDetail
              key={selected.id}
              network={selected}
              deviceCount={counts.get(selected.id) ?? 0}
              notice={notice?.id === selected.id ? notice.text : null}
              onShowDevices={() => onShowDevices(selected.id)}
              onRemove={() => setRemoving(selected)}
            />
          )}
        </div>
      )}

      <AddNetworkDialog
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onAdded={(n, followUp) => {
          setAddOpen(false);
          onSelect(n.id);
          setNotice(followUp ? { id: n.id, text: followUp } : null);
        }}
      />
      <RemoveNetworkDialog
        network={removing}
        onClose={() => setRemoving(null)}
        onRemoved={() => {
          setRemoving(null);
          onSelect(null);
        }}
      />
    </div>
  );
}
