"use client";

import { motion, useReducedMotion, type HTMLMotionProps } from "framer-motion";
import type { ReactNode } from "react";

/** Small, interruptible feedback; native button semantics and focus stay intact. */
export function ApplicationButton({
  children,
  className = "",
  disabled,
  pending = false,
  ...props
}: Omit<HTMLMotionProps<"button">, "children"> & { children: ReactNode; pending?: boolean }) {
  const reducedMotion = useReducedMotion();
  const inactive = disabled || pending;

  return (
    <motion.button
      {...props}
      aria-busy={pending || undefined}
      className={`button application-motion-button ${className}`}
      disabled={inactive}
      animate={{ y: 0, scale: 1 }}
      whileHover={!inactive && !reducedMotion ? { y: -2 } : undefined}
      whileTap={!inactive && !reducedMotion ? { y: 0, scale: 0.98 } : undefined}
      transition={{ duration: reducedMotion ? 0 : 0.14, ease: "easeOut" }}
    >
      {pending ? <span aria-hidden="true" className="application-busy-indicator" /> : null}
      {children}
    </motion.button>
  );
}
