import { auth } from "@clerk/nextjs/server";
import { redirect } from "next/navigation";
import { OrganizerEventOperations } from "@/components/organizer-event-operations";
import {
  ApiError,
  createApiClient,
  type OrganizerActivityListResponse,
  type OrganizerCheckpointListResponse,
  type OrganizerRedemptionCountsResponse,
} from "@/lib/api";
import { StaffErrorState, StaffPageFrame } from "@/components/staff-workflow";

type OrganizerOperationsData = {
  activities: OrganizerActivityListResponse;
  checkpoints: OrganizerCheckpointListResponse;
  counts: OrganizerRedemptionCountsResponse;
};

function organizerOperationsLoadMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) {
      return "Your session has ended. Sign in again to access event operations.";
    }

    if (error.status === 403) {
      return "Admin access is required to manage event operations.";
    }
  }

  return error instanceof Error
    ? error.message
    : "Event operations could not be loaded.";
}

export default async function OrganizerOperationsPage() {
  const { userId, getToken } = await auth();
  if (!userId) {
    redirect("/");
  }

  const client = createApiClient({ getToken });
  let operations: OrganizerOperationsData | null = null;
  let loadError: unknown = undefined;

  try {
    const [activities, checkpoints, counts] = await Promise.all([
      client.listOrganizerActivities(),
      client.listOrganizerCheckpoints(),
      client.listOrganizerRedemptionCounts(),
    ]);
    operations = { activities, checkpoints, counts };
  } catch (error) {
    loadError = error;
  }

  if (!operations) {
    return (
      <StaffPageFrame
        eyebrow="Admin workspace"
        role="admin"
        title="Event operations"
      >
        <StaffErrorState title="Event operations unavailable">
          {organizerOperationsLoadMessage(loadError)}
        </StaffErrorState>
      </StaffPageFrame>
    );
  }

  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Event operations"
    >
      <OrganizerEventOperations
        initialActivities={operations.activities.items}
        initialCheckpoints={operations.checkpoints.items}
        initialCounts={operations.counts.items}
      />
    </StaffPageFrame>
  );
}
