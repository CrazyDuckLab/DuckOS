/**
 * Tiny Link wrapper used by the Easy mode guided workflows.
 *
 * Renders a normal anchor with `href` so middle-click / right-click /
 * cmd-click work like a real link. Adds a `data-v1-link` attribute so
 * future router integration can intercept clicks.
 */
import type { ReactNode } from "react";

export function Link({ to, children, ...rest }: {
  to: string;
  children: ReactNode;
  className?: string;
  ["data-variant"]?: string;
}) {
  return (
    <a href={to} data-v1-link={to} {...rest}>
      {children}
    </a>
  );
}