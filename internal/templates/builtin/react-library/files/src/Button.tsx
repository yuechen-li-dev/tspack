import type { ReactNode } from "react";

export interface ButtonProps {
  children: ReactNode;
  variant?: "primary" | "secondary";
}

export function Button({ children, variant = "primary" }: ButtonProps) {
  return (
    <button className={`tspack-button tspack-button--${variant}`} type="button">
      {children}
    </button>
  );
}
