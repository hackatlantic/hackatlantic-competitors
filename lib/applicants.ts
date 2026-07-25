import "server-only";
import { randomUUID } from "crypto";
import { getSupabaseAdmin } from "@/lib/supabase/admin";

export type Applicant = {
  user_id: string;
  accepted: boolean;
  qr_code_id: string | null;
  full_name: string | null;
  email: string | null;
  school: string | null;
  experience: string | null;
  goals: string | null;
  resume_path: string | null;
  applied_at: string | null;
  created_at?: string;
  updated_at?: string;
};

export async function listApplicants() {
  const supabaseAdmin = getSupabaseAdmin();
  const { data, error } = await supabaseAdmin
    .from("applicants")
    .select("*")
    .order("applied_at", { ascending: false, nullsFirst: false });

  if (error) {
    throw error;
  }

  return data as Applicant[];
}

export async function getOrCreateApplicant(userId: string) {
  const supabaseAdmin = getSupabaseAdmin();
  const { data: existingApplicant, error: selectError } = await supabaseAdmin
    .from("applicants")
    .select("*")
    .eq("user_id", userId)
    .maybeSingle<Applicant>();

  if (selectError) {
    throw selectError;
  }

  if (existingApplicant) {
    return existingApplicant;
  }

  const { data: newApplicant, error: insertError } = await supabaseAdmin
    .from("applicants")
    .insert({ user_id: userId, accepted: false })
    .select("*")
    .single<Applicant>();

  if (insertError) {
    throw insertError;
  }

  return newApplicant;
}

export async function ensureQrCodeId(applicant: Applicant) {
  if (!applicant.accepted) {
    return null;
  }

  if (applicant.qr_code_id) {
    return applicant.qr_code_id;
  }

  const qrCodeId = `ha_${randomUUID()}`;
  const supabaseAdmin = getSupabaseAdmin();

  const { data, error } = await supabaseAdmin
    .from("applicants")
    .update({ qr_code_id: qrCodeId })
    .eq("user_id", applicant.user_id)
    .select("qr_code_id")
    .single<{ qr_code_id: string }>();

  if (error) {
    throw error;
  }

  return data.qr_code_id;
}
