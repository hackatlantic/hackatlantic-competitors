"use client";

import { useMemo } from "react";
import { BarcodeFormat, EncodeHintType, QRCodeWriter } from "@zxing/library";

export function PassQrCode({ value }: { value: string }) {
  const code = useMemo(() => {
    // Keep credentials bounded; never include the payload in errors or markup labels.
    if (!value || new TextEncoder().encode(value).length > 106) return null;
    try {
      const hints = new Map<EncodeHintType, string | number>([
        [EncodeHintType.ERROR_CORRECTION, "M"],
        [EncodeHintType.MARGIN, 4],
        [EncodeHintType.QR_VERSION, 5],
      ]);
      const matrix = new QRCodeWriter().encode(value, BarcodeFormat.QR_CODE, 0, 0, hints);
      const paths: string[] = [];
      for (let y = 0; y < matrix.getHeight(); y++) {
        for (let x = 0; x < matrix.getWidth(); x++) {
          if (matrix.get(x, y)) paths.push(`M${x} ${y}h1v1h-1z`);
        }
      }
      return { size: matrix.getWidth(), path: paths.join("") };
    } catch {
      return null;
    }
  }, [value]);

  if (!code) return <p className="pass-qr-unavailable" role="alert">Your pass code could not be displayed. Ask an organizer to reissue your pass.</p>;
  return (
    <svg aria-label="QR code for your HackAtlantic entry pass" className="pass-qr-code" role="img" shapeRendering="crispEdges" viewBox={`0 0 ${code.size} ${code.size}`} xmlns="http://www.w3.org/2000/svg">
      <rect fill="#ffffff" width={code.size} height={code.size} />
      <path d={code.path} fill="#111827" />
    </svg>
  );
}
