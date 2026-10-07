import type { ButtonHTMLAttributes } from "react";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger" | "danger-solid";
  loading?: boolean;
};

export function Button({ variant = "secondary", loading = false, disabled, className, children, ...rest }: Props) {
  const cls = ["btn", variant !== "secondary" && `btn-${variant}`, className].filter(Boolean).join(" ");
  return (
    <button type="button" className={cls} disabled={disabled || loading} aria-busy={loading || undefined} {...rest}>
      {loading && <span className="spinner" aria-hidden="true" />}
      {children}
    </button>
  );
}
