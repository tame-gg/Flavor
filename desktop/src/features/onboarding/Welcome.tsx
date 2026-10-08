import logo from "../../logo.svg";
import { Button } from "../../components/ui/Button";

export function Welcome({ onAdd, onLater }: { onAdd: () => void; onLater: () => void }) {
  return (
    <main className="center-screen">
      <div className="center-card">
        <img className="logo" src={logo} alt="" />
        <h1>Welcome to Flavor</h1>
        <p className="muted">
          Join Tailscale and Headscale networks side by side. Each network gets its own isolated session and device
          identity, so overlapping addresses never get mixed up.
        </p>
        <p className="muted">Nothing on this computer changes until you add a network.</p>
        <div className="row">
          <Button variant="primary" onClick={onAdd}>
            Add your first network
          </Button>
          <Button variant="ghost" onClick={onLater}>
            Set up later
          </Button>
        </div>
      </div>
    </main>
  );
}
