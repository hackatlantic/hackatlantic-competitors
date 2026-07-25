import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "HackAtlantic Competitors",
  description: "A Next.js starter app."
};

export default function RootLayout({
  children
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
