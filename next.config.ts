import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Vercel packages the Next.js runtime itself and expects the default trace
  // layout. Container builds still need the standalone server artifact.
  ...(process.env.VERCEL ? {} : { output: "standalone" as const }),
};

export default nextConfig;
