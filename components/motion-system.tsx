"use client";

import { usePathname } from "next/navigation";
import {
  MotionConfig,
  motion,
  useMotionValue,
  useReducedMotion,
  useSpring,
  useTransform,
  useVelocity,
} from "framer-motion";
import { useEffect, useState, type ReactNode } from "react";

type MotionSystemProps = {
  children: ReactNode;
};

function ElasticCursor() {
  const reducedMotion = useReducedMotion();
  const [enabled, setEnabled] = useState(false);
  const [interactive, setInteractive] = useState(false);
  const cursorX = useMotionValue(-80);
  const cursorY = useMotionValue(-80);
  const springX = useSpring(cursorX, { stiffness: 620, damping: 38, mass: 0.32 });
  const springY = useSpring(cursorY, { stiffness: 620, damping: 38, mass: 0.32 });
  const velocityX = useVelocity(springX);
  const stretch = useTransform(velocityX, [-1800, 0, 1800], [1.65, 1, 1.65]);
  const squash = useTransform(velocityX, [-1800, 0, 1800], [0.72, 1, 0.72]);

  useEffect(() => {
    const preciseDesktop = window.matchMedia(
      "(pointer: fine) and (hover: hover) and (min-width: 1024px)",
    );

    const updateAvailability = () => {
      setEnabled(preciseDesktop.matches && !reducedMotion);
    };

    updateAvailability();
    preciseDesktop.addEventListener("change", updateAvailability);
    return () => preciseDesktop.removeEventListener("change", updateAvailability);
  }, [reducedMotion]);

  useEffect(() => {
    if (!enabled) {
      return;
    }

    const moveCursor = (event: PointerEvent) => {
      cursorX.set(event.clientX);
      cursorY.set(event.clientY);
      const target = event.target instanceof Element ? event.target : null;
      setInteractive(
        Boolean(
          target?.closest(
            "a, button, input, textarea, select, summary, [data-cursor-target]",
          ),
        ),
      );
    };

    window.addEventListener("pointermove", moveCursor, { passive: true });
    return () => window.removeEventListener("pointermove", moveCursor);
  }, [cursorX, cursorY, enabled]);

  if (!enabled) {
    return null;
  }

  return (
    <motion.div
      aria-hidden="true"
      className="elastic-cursor-anchor"
      style={{
        x: springX,
        y: springY,
      }}
    >
      <motion.div
        animate={{
          height: interactive ? 42 : 22,
          opacity: 1,
          width: interactive ? 42 : 22,
        }}
        className="elastic-cursor"
        initial={{ opacity: 0 }}
        style={{
          scaleX: interactive ? 1 : stretch,
          scaleY: interactive ? 1 : squash,
        }}
        transition={{ type: "spring", stiffness: 520, damping: 30 }}
      />
    </motion.div>
  );
}

export function MotionSystem({ children }: MotionSystemProps) {
  const pathname = usePathname();

  return (
    <MotionConfig reducedMotion="user">
      <div className="static-noise" aria-hidden="true" />
      <ElasticCursor />
      <motion.div
        animate={{ opacity: 1, y: 0 }}
        className="route-motion-shell"
        initial={{ opacity: 0, y: 10 }}
        key={pathname}
        transition={{ duration: 0.28, ease: [0.22, 1, 0.36, 1] }}
      >
        {children}
      </motion.div>
    </MotionConfig>
  );
}
