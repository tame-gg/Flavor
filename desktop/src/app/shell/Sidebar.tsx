import {
  Activity,
  Boxes,
  LayoutDashboard,
  type LucideIcon,
  Monitor,
  Network as NetworkIcon,
  Route,
  Search,
  Settings,
  TriangleAlert,
} from "lucide-react";
import { motion } from "motion/react";
import logo from "../../logo.svg";
import { cn } from "@/lib/cn";
import { modKey } from "@/lib/platform";
import type { Page } from "../navigation";
import type { DaemonStatus } from "../sync/controller";

const icons: Record<Page, LucideIcon> = {
  overview: LayoutDashboard,
  networks: NetworkIcon,
  workspaces: Boxes,
  devices: Monitor,
  inspector: Route,
  conflicts: TriangleAlert,
  diagnostics: Activity,
  settings: Settings,
};

const statusText: Record<DaemonStatus, [string, string]> = {
  ready: ["Connected to flavord", "bg-success"],
  starting: ["Connecting to flavord", "bg-warning"],
  resyncing: ["Refreshing", "bg-warning"],
  unavailable: ["flavord not reachable, retrying", "bg-destructive"],
  incompatible: ["Incompatible flavord", "bg-destructive"],
};

type Props = {
  pages: [Page, string][];
  page: Page;
  onPage: (p: Page) => void;
  counts: Partial<Record<Page, number>>;
  alerts: Partial<Record<Page, number>>;
  workspace: string | null;
  status: DaemonStatus;
  onSearch: () => void;
};

export function Sidebar({ pages, page, onPage, counts, alerts, workspace, status, onSearch }: Props) {
  const [label, dot] = statusText[status];
  const main = pages.filter(([id]) => id !== "settings");
  const settings = pages.find(([id]) => id === "settings");
  return (
    <aside className="flex w-56 shrink-0 flex-col gap-1 border-r border-border bg-card px-3 py-4">
      <div className="flex items-center gap-2 px-2 pb-3 font-semibold">
        <img src={logo} alt="" className="size-6" />
        Flavor
      </div>
      <button
        type="button"
        onClick={onSearch}
        className="mb-2 flex h-8 items-center gap-2 rounded-md border border-border bg-background px-2 text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
      >
        <Search className="size-4" />
        <span className="flex-1 text-left whitespace-nowrap">Search</span>
        <kbd className="rounded border border-border px-1 font-mono text-[11px]">{modKey} K</kbd>
      </button>
      <nav aria-label="Sections" className="flex flex-col gap-0.5">
        {main.map(([id, text]) => (
          <NavItem
            key={id}
            id={id}
            text={text}
            active={page === id}
            count={counts[id]}
            alert={alerts[id]}
            onPage={onPage}
          />
        ))}
      </nav>
      <div className="mt-auto flex flex-col gap-1">
        {workspace && (
          <button
            type="button"
            onClick={() => onPage("workspaces")}
            className="flex flex-col rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
          >
            <span className="text-xs text-muted-foreground">Workspace</span>
            <span className="text-sm font-medium">{workspace}</span>
          </button>
        )}
        {settings && <NavItem id="settings" text={settings[1]} active={page === "settings"} onPage={onPage} />}
        <div
          className="flex items-center gap-2 border-t border-border px-2 pt-3 text-xs text-muted-foreground"
          role="status"
        >
          <span className={cn("size-2 shrink-0 rounded-full", dot)} />
          {label}
        </div>
      </div>
    </aside>
  );
}

function NavItem({
  id,
  text,
  active,
  count,
  alert,
  onPage,
}: {
  id: Page;
  text: string;
  active: boolean;
  count?: number;
  alert?: number;
  onPage: (p: Page) => void;
}) {
  const Icon = icons[id];
  return (
    <button
      type="button"
      aria-current={active ? "page" : undefined}
      onClick={() => onPage(id)}
      className={cn(
        "relative flex h-8 items-center gap-2 rounded-md px-2 text-sm transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none",
        active ? "text-foreground" : "text-muted-foreground hover:bg-accent hover:text-foreground",
      )}
    >
      {active && (
        <motion.span
          layoutId="nav-active"
          className="absolute inset-0 rounded-md bg-accent"
          transition={{ type: "spring", stiffness: 500, damping: 40 }}
        />
      )}
      <Icon className="relative size-4" />
      <span className="relative flex-1 text-left">{text}</span>
      {alert ? (
        <span className="relative rounded-full bg-warning/15 px-1.5 text-[11px] leading-4 font-medium text-warning">
          {alert}
        </span>
      ) : count !== undefined ? (
        <span className="relative font-mono text-[11px] text-muted-foreground">{count}</span>
      ) : null}
    </button>
  );
}
