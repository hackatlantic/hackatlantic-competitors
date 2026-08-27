import type { Metadata } from "next";
import { ClerkProvider } from "@clerk/nextjs";
import { MotionSystem } from "@/components/motion-system";
import "./globals.css";

export const metadata: Metadata = {
  title: "Apply · HackAtlantic",
  description: "HackAtlantic application portal."
};

export default function RootLayout({
  children
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <ClerkProvider>
          <MotionSystem>{children}</MotionSystem>
        </ClerkProvider>
      </body>
    </html>
  );
}
