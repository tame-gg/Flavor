import type { ReactNode } from "react";

export function Banner({ tone = "info", children }: { tone?: "info" | "warn" | "danger"; children: ReactNode }) {
  return (
    <div className={`banner ${tone === "info" ? "" : `banner-${tone}`}`} role={tone === "danger" ? "alert" : "status"}>
      {children}
    </div>
  );
}
