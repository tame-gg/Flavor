import { CircleCheckIcon, InfoIcon, Loader2Icon, OctagonXIcon, TriangleAlertIcon } from "lucide-react";
import type { CSSProperties } from "react";
import { Toaster as Sonner, type ToasterProps } from "sonner";

const Toaster = ({ ...props }: ToasterProps) => {
  const theme = document.documentElement.dataset.theme;
  return (
    <Sonner
      theme={theme === "light" || theme === "dark" ? theme : "system"}
      className="toaster group"
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      style={
        {
          "--normal-bg": "var(--ui-surface)",
          "--normal-text": "var(--ui-foreground)",
          "--normal-border": "var(--ui-border)",
          "--border-radius": "var(--ui-radius)",
        } as CSSProperties
      }
      {...props}
    />
  );
};

export { Toaster };
