import type { Metadata } from "next";
import { ClerkProvider } from "@clerk/nextjs";
import { MotionSystem } from "@/components/motion-system";
import "./globals.css";
import "./brand.css";

export const metadata: Metadata = {
  title: "Apply · HackAtlantic",
  description: "Apply to HackAtlantic and manage your application, decision, and event pass."
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
