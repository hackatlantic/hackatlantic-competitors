"use server";

import { auth } from "@clerk/nextjs/server";
import { redirect } from "next/navigation";
import { randomUUID } from "crypto";
import { supabaseAdmin } from "@/lib/supabase/admin";

const MAX_RESUME_SIZE_BYTES = 5 * 1024 * 1024;

export async function submitApplication(formData: FormData) {
  const { userId } = await auth();

  if (!userId) {
    redirect("/");
  }

  const fullName = getRequiredString(formData, "fullName");
  const email = getRequiredString(formData, "email");
  const school = getRequiredString(formData, "school");
  const experience = getRequiredString(formData, "experience");
  const goals = getRequiredString(formData, "goals");
  const resume = formData.get("resume");

  if (!(resume instanceof File) || resume.size === 0) {
    throw new Error("Resume is required.");
  }

  if (resume.size > MAX_RESUME_SIZE_BYTES) {
    throw new Error("Resume must be 5MB or smaller.");
  }

  const resumePath = `${userId}/${randomUUID()}-${sanitizeFileName(resume.name)}`;
  const uploadBody = Buffer.from(await resume.arrayBuffer());

  const { error: uploadError } = await supabaseAdmin.storage
    .from("resumes")
    .upload(resumePath, uploadBody, {
      contentType: resume.type || "application/octet-stream",
      upsert: false
    });

  if (uploadError) {
    throw uploadError;
  }

  const { error } = await supabaseAdmin.from("applicants").upsert({
    user_id: userId,
    accepted: false,
    full_name: fullName,
    email,
    school,
    experience,
    goals,
    resume_path: resumePath,
    applied_at: new Date().toISOString()
  });

  if (error) {
    throw error;
  }

  redirect("/");
}

function getRequiredString(formData: FormData, key: string) {
  const value = formData.get(key);

  if (typeof value !== "string" || value.trim().length === 0) {
    throw new Error(`${key} is required.`);
  }

  return value.trim();
}

function sanitizeFileName(fileName: string) {
  return fileName.replace(/[^a-zA-Z0-9._-]/g, "-");
}
