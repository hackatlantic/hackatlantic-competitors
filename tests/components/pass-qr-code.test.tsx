import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { BinaryBitmap, HybridBinarizer, QRCodeReader, RGBLuminanceSource } from "@zxing/library";
import { PassQrCode } from "@/components/pass-qr-code";

describe("Pass QR encoding", () => {
  afterEach(cleanup);
  it.each([4, 6])("round-trips credential-shaped payloads at %i pixels per module", (scale) => {
    for (const suffix of ["aB3_-Cd9".repeat(5) + "xyz", "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdef"]) {
      const token = `qr_v1.${suffix}`;
      const view = render(<PassQrCode value={token} />);
      const svg = screen.getByRole("img", { name: /QR code/ });
      const size = Number(svg.getAttribute("viewBox")?.split(" ")[2]);
      const pixels = new Uint8ClampedArray(size * size * scale * scale).fill(255);
      const width = size * scale;
      const path = svg.querySelector("path")!.getAttribute("d")!;
      for (const match of path.matchAll(/M(\d+) (\d+)h1v1h-1z/g)) {
        const x = Number(match[1]) * scale;
        const y = Number(match[2]) * scale;
        for (let dy = 0; dy < scale; dy++) for (let dx = 0; dx < scale; dx++) pixels[(y + dy) * width + x + dx] = 0;
      }
      const result = new QRCodeReader().decode(new BinaryBitmap(new HybridBinarizer(new RGBLuminanceSource(pixels, width, width))));
      expect(result.getText()).toBe(token);
      view.unmount();
    }
  });

  it.each(["", "x".repeat(107)])("fails safely for invalid input", (value) => {
    render(<PassQrCode value={value} />);
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.queryByRole("img")).toBeNull();
  });
});
