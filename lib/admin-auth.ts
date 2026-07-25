import "server-only";
import { currentUser } from "@clerk/nextjs/server";

export async function requireAdmin() {
  const user = await currentUser();

  if (!user) {
    return { authorized: false, reason: "signed-out" as const };
  }

  const adminEmails = getAdminEmails();
  const userEmails = user.emailAddresses.map((email) =>
    email.emailAddress.toLowerCase()
  );
  const isAdmin = userEmails.some((email) => adminEmails.has(email));

  return {
    authorized: isAdmin,
    reason: isAdmin ? "authorized" as const : "forbidden" as const,
    user
  };
}

function getAdminEmails() {
  return new Set(
    (process.env.ADMIN_EMAILS ?? "")
      .split(",")
      .map((email) => email.trim().toLowerCase())
      .filter(Boolean)
  );
}
