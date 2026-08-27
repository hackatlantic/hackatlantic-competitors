import {
  Show,
  UserButton,
} from "@clerk/nextjs";
import { ApplicantDashboard } from "@/components/applicant-dashboard";
import { ApplicationEntry } from "@/components/application-entry";
import { RoleNavigation } from "@/components/role-navigation";
import { BrandMark } from "@/components/brand-mark";

export default function Home() {
  return (
    <main className="page portal-page">
      <Show when="signed-in">
        <header className="portal-header">
          <BrandMark />
          <nav className="nav" aria-label="Account">
            <UserButton />
          </nav>
        </header>
      </Show>

      <Show when="signed-out">
        <div className="signed-in-home portal-workspace application-flow-workspace">
          <ApplicationEntry requiresAuth />
        </div>
      </Show>

      <Show when="signed-in">
        <div className="signed-in-home portal-workspace application-flow-workspace">
          <RoleNavigation />
          <ApplicantDashboard />
        </div>
      </Show>
    </main>
  );
}
