import { NextRequest } from "next/server";
import { requireAdmin } from "@/lib/admin-auth";
import { getSupabaseAdmin } from "@/lib/supabase/admin";

export const dynamic = "force-dynamic";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const admin = await requireAdmin();

  if (!admin.authorized) {
    return new Response("Not found", { status: 404 });
  }

  const { path } = await params;
  const resumePath = path.join("/");

  if (!resumePath || resumePath.includes("..")) {
    return new Response("Invalid resume path", { status: 400 });
  }

  const supabaseAdmin = getSupabaseAdmin();
  const { data, error } = await supabaseAdmin.storage
    .from("resumes")
    .download(resumePath);

  if (error || !data) {
    return new Response("Resume not found", { status: 404 });
  }

  return new Response(data, {
    headers: {
      "Cache-Control": "private, no-store",
      "Content-Disposition": `inline; filename="${getFileName(resumePath)}"`,
      "Content-Type": data.type || "application/octet-stream"
    }
  });
}

function getFileName(path: string) {
  return path.split("/").at(-1)?.replace(/"/g, "") || "resume";
}
