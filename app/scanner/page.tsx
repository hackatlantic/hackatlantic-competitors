import { auth } from "@clerk/nextjs/server";
import { redirect } from "next/navigation";
import { ScannerWorkflow } from "@/components/scanner-workflow";

export default async function ScannerPage() {
  const { userId } = await auth();
  if (!userId) {
    redirect("/");
  }

  return <ScannerWorkflow />;
}
