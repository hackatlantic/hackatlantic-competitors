const DEFAULT_API_BASE_URL = "http://localhost:8080";

export type ApiErrorBody = {
  code?: string;
  message?: string;
  requestId?: string;
  details?: unknown;
};

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: ApiErrorBody,
  ) {
    super(body.message ?? `API request failed with status ${status}`);
    this.name = "ApiError";
  }
}

type TokenProvider = () => Promise<string | null>;
type ApiPath = `/${string}`;

export type ApiClientOptions = {
  baseUrl?: string;
  getToken?: TokenProvider;
};

export type CurrentUserRole =
  | "applicant"
  | "admin"
  | "scanner";

export type CurrentUser = {
  id: string;
  email: string;
  displayName: string | null;
  roles: CurrentUserRole[];
};

export type ApplicationQuestionType = "string" | "number" | "boolean";

export type ApplicationFormQuestion = {
  key: string;
  label: string;
  type: ApplicationQuestionType;
  required: boolean;
  help?: string;
  maxWords?: number;
};

export type CurrentApplicationForm = {
  id: string;
  cycleId: string;
  version: number;
  resumeRequired: boolean;
  questions: ApplicationFormQuestion[];
};

export type ApplicationResume = {
  applicationId: string;
  originalFilename: string;
  mediaType: "application/pdf";
  byteSize: number;
  uploadedAt: string;
  updatedAt: string;
};

export type ApplicationAnswer = string | number | boolean;
export type ApplicationAnswers = Record<string, ApplicationAnswer>;

export type ApplicantApplication = {
  id: string;
  cycleId: string;
  formId: string;
  formVersion: number;
  status: "draft" | "submitted";
  submittedAt?: string;
  lockVersion: number;
  answers: ApplicationAnswers;
  createdAt: string;
  updatedAt: string;
};

export type SaveApplicationDraftRequest = {
  lockVersion: number;
  answers: ApplicationAnswers;
};

export type MyApplicationsResponse = {
  items: ApplicantApplication[];
  nextCursor: string | null;
};

export type SubmitApplicationRequest = {
  lockVersion: number;
};
export type DecisionOutcome = "accepted" | "waitlisted" | "rejected";

export type RecordDecisionRequest = {
  outcome: DecisionOutcome;
  internalReason?: string;
};

export type OrganizerDecision = {
  id: string;
  applicationId: string;
  outcome: DecisionOutcome;
  internalReason?: string;
  decidedBy: string;
  decidedAt: string;
  supersedesId?: string;
  releasedBy?: string;
  releasedAt?: string;
};

export type ApplicantReleasedDecision = {
  applicationId: string;
  outcome: DecisionOutcome;
  releasedAt: string;
};

export type OrganizerApplicationStatus =
  | "submitted"
  | "accepted"
  | "waitlisted"
  | "rejected";

export type StaffApplicant = {
  id: string;
  email: string;
  displayName: string | null;
};

export type PassStatus = "active" | "revoked";

export type AttendeePass = {
  id: string;
  attendeeId: string;
  displayName: string;
  status: PassStatus;
  issuedAt: string;
  revokedAt?: string;
};

export type AuthenticatedAttendeePass = Omit<
  AttendeePass,
  "status" | "revokedAt"
> & {
  status: "active";
  qrToken: string;
};

export type PassIssuance = AttendeePass & {
  qrToken: string;
  claimToken: string;
  claimUrl: string;
};

export type OrganizerAttendeePass = {
  attendeeId: string;
  pass: AttendeePass | null;
};

export type ScannerCheckpoint = {
  id: string;
  name: string;
};

export type ScannerCheckpointListResponse = {
  items: ScannerCheckpoint[];
  nextCursor: string | null;
};

export type ScannerAttendee = {
  displayName: string;
};

export type ScannerPassVerification = {
  status: "active" | "revoked";
};

export type ScannerLookupRequest = {
  qrToken: string;
};

export type ScannerLookupResponse = {
  attendee: ScannerAttendee;
  pass: ScannerPassVerification;
};

export type ScannerRedemptionOutcome =
  | "redeemed"
  | "already_exhausted"
  | "not_entitled"
  | "outside_window"
  | "invalid_pass"
  | "revoked_pass";

export type ScannerRedemptionRequest = {
  qrToken: string;
  checkpointId: string;
  idempotencyKey: string;
};

export type ScannerRedemptionResponse = {
  outcome: ScannerRedemptionOutcome;
  attendee?: ScannerAttendee;
  pass?: ScannerPassVerification;
  checkpoint?: ScannerCheckpoint;
  redemptionId?: string;
};

export type OrganizerActivity = {
  id: string;
  cycleId: string;
  slug: string;
  name: string;
  startsAt?: string | null;
  endsAt?: string | null;
  createdAt?: string;
  updatedAt?: string;
};

export type OrganizerActivityListResponse = {
  items: OrganizerActivity[];
};

export type CreateOrganizerActivityRequest = {
  cycleId: string;
  slug: string;
  name: string;
  startsAt?: string;
  endsAt?: string;
};

export type UpdateOrganizerActivityRequest = Omit<
  CreateOrganizerActivityRequest,
  "cycleId" | "startsAt" | "endsAt"
> & {
  startsAt?: string | null;
  endsAt?: string | null;
};

export type OrganizerCheckpoint = {
  id: string;
  cycleId: string;
  activityId?: string | null;
  slug: string;
  name: string;
  opensAt?: string | null;
  closesAt?: string | null;
  defaultAllowed: boolean;
  defaultMaxRedemptions: number;
  active: boolean;
  createdAt?: string;
  updatedAt?: string;
};

export type OrganizerCheckpointListResponse = {
  items: OrganizerCheckpoint[];
};

export type CreateOrganizerCheckpointRequest = {
  cycleId: string;
  activityId?: string | null;
  slug: string;
  name: string;
  opensAt?: string;
  closesAt?: string;
  defaultAllowed: boolean;
  defaultMaxRedemptions: number;
  active: boolean;
};

export type UpdateOrganizerCheckpointRequest = Omit<
  CreateOrganizerCheckpointRequest,
  "cycleId" | "opensAt" | "closesAt"
> & {
  opensAt?: string | null;
  closesAt?: string | null;
};

export type OrganizerEntitlement = {
  attendeeId: string;
  checkpointId: string;
  allowed: boolean;
  maxRedemptions: number;
  createdAt?: string;
  updatedAt?: string;
};

export type OrganizerAttendeeEntitlementResponse = {
  override: OrganizerEntitlement | null;
};

export type UpdateOrganizerEntitlementRequest = {
  allowed: boolean;
  maxRedemptions: number;
};

export type OrganizerRedemptionCount = {
  checkpointId: string;
  checkpointName: string;
  totalRedemptions: number;
  lastRedeemedAt?: string | null;
};

export type OrganizerRedemptionCountsResponse = {
  items: OrganizerRedemptionCount[];
};

export type OrganizerRedemption = {
  id: string;
  redeemedAt: string;
  ordinal: number;
  checkpoint: Pick<OrganizerCheckpoint, "id" | "slug" | "name">;
  attendee: {
    id: string;
    displayName: string;
  };
  pass: {
    id: string;
    status: "active" | "revoked" | "replaced";
  };
  scannerUserId: string;
};

export type OrganizerRedemptionListResponse = {
  items: OrganizerRedemption[];
};

export type OrganizerExportFilters = {
  checkpointId?: string;
};

export type OrganizerCsvDownload = {
  blob: Blob;
  filename: string | null;
};

export type OrganizerApplication = {
  id: string;
  cycleId: string;
  formId: string;
  formVersion: number;
  status: OrganizerApplicationStatus;
  submittedAt?: string;
  applicant: StaffApplicant;
  answers: ApplicationAnswers;
  currentDecision?: OrganizerDecision;
  attendeePass?: OrganizerAttendeePass;
  createdAt: string;
  updatedAt: string;
};

export type OrganizerApplicationListResponse = {
  items: OrganizerApplication[];
  nextCursor: string | null;
};

export type OrganizerApplicationFilters = {
  status?: OrganizerApplicationStatus;
  q?: string;
};

export type AssignReviewerRequest = {
  reviewerUserId: string;
};

export type ReviewerAssignmentResult = {
  applicationId: string;
  reviewerUserId: string;
};

export type ReviewerRecommendation =
  | "strong_yes"
  | "yes"
  | "neutral"
  | "no"
  | "strong_no";

export type ReviewScore = 1 | 2 | 3 | 4 | 5;

export type ReviewerApplicationAssignment = {
  assignedBy: string;
  assignedAt: string;
};

export type ReviewerReview = {
  id: string;
  status: "draft" | "submitted";
  score: ReviewScore;
  recommendation: ReviewerRecommendation;
  internalNotes?: string;
  lockVersion: number;
  submittedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type ReviewerApplication = {
  id: string;
  cycleId: string;
  formId: string;
  formVersion: number;
  status: "submitted";
  submittedAt: string;
  applicant: StaffApplicant;
  answers: ApplicationAnswers;
  assignment?: ReviewerApplicationAssignment | null;
  review?: ReviewerReview | null;
  createdAt: string;
  updatedAt: string;
};

export type ReviewerApplicationListResponse = {
  items: ReviewerApplication[];
  nextCursor: string | null;
};

export type SaveReviewDraftRequest = {
  lockVersion: number;
  score: ReviewScore;
  recommendation: ReviewerRecommendation;
  internalNotes?: string;
};

export type ApiClient = {
  request<TResponse>(path: ApiPath, init?: RequestInit): Promise<TResponse>;
  getCurrentUser(): Promise<CurrentUser>;
  getCurrentApplicationForm(): Promise<CurrentApplicationForm>;
  createApplication(): Promise<ApplicantApplication>;
  getMyApplications(): Promise<MyApplicationsResponse>;
  getApplicationDecision(applicationId: string): Promise<ApplicantReleasedDecision>;
  saveApplicationDraft(
    applicationId: string,
    request: SaveApplicationDraftRequest,
  ): Promise<ApplicantApplication>;
  submitApplication(
    applicationId: string,
    request: SubmitApplicationRequest,
  ): Promise<ApplicantApplication>;
  getApplicationResume(applicationId: string): Promise<ApplicationResume>;
  uploadApplicationResume(
    applicationId: string,
    file: File,
  ): Promise<ApplicationResume>;
  downloadAdminApplicationResume(applicationId: string): Promise<Blob>;
  listOrganizerApplications(
    filters?: OrganizerApplicationFilters,
  ): Promise<OrganizerApplicationListResponse>;
  getOrganizerApplication(applicationId: string): Promise<OrganizerApplication>;
  getAttendeePass(): Promise<AuthenticatedAttendeePass>;
  issueAttendeePass(attendeeId: string): Promise<PassIssuance>;
  revokeAttendeePass(passId: string): Promise<AttendeePass>;
  reissueAttendeePass(passId: string): Promise<PassIssuance>;
  listScannerCheckpoints(
    cursor?: string,
  ): Promise<ScannerCheckpointListResponse>;
  lookupScannerPass(request: ScannerLookupRequest): Promise<ScannerLookupResponse>;
  redeemScannerPass(
    request: ScannerRedemptionRequest,
  ): Promise<ScannerRedemptionResponse>;
  listOrganizerActivities(): Promise<OrganizerActivityListResponse>;
  createOrganizerActivity(
    request: CreateOrganizerActivityRequest,
  ): Promise<OrganizerActivity>;
  updateOrganizerActivity(
    activityId: string,
    request: UpdateOrganizerActivityRequest,
  ): Promise<OrganizerActivity>;
  deleteOrganizerActivity(activityId: string): Promise<void>;
  listOrganizerCheckpoints(): Promise<OrganizerCheckpointListResponse>;
  createOrganizerCheckpoint(
    request: CreateOrganizerCheckpointRequest,
  ): Promise<OrganizerCheckpoint>;
  updateOrganizerCheckpoint(
    checkpointId: string,
    request: UpdateOrganizerCheckpointRequest,
  ): Promise<OrganizerCheckpoint>;
  deleteOrganizerCheckpoint(checkpointId: string): Promise<void>;
  getOrganizerAttendeeEntitlement(
    attendeeId: string,
    checkpointId: string,
  ): Promise<OrganizerAttendeeEntitlementResponse>;
  updateOrganizerAttendeeEntitlement(
    attendeeId: string,
    checkpointId: string,
    request: UpdateOrganizerEntitlementRequest,
  ): Promise<OrganizerEntitlement>;
  deleteOrganizerAttendeeEntitlement(
    attendeeId: string,
    checkpointId: string,
  ): Promise<void>;
  listOrganizerRedemptionCounts(): Promise<OrganizerRedemptionCountsResponse>;
  listOrganizerRedemptions(): Promise<OrganizerRedemptionListResponse>;
  downloadOrganizerAttendanceCsv(
    filters?: OrganizerExportFilters,
  ): Promise<OrganizerCsvDownload>;
  downloadOrganizerReconciliationCsv(
    filters?: OrganizerExportFilters,
  ): Promise<OrganizerCsvDownload>;
  recordOrganizerDecision(
    applicationId: string,
    request: RecordDecisionRequest,
  ): Promise<OrganizerDecision>;
  releaseOrganizerDecision(decisionId: string): Promise<OrganizerDecision>;
  grantReviewerRole(userId: string): Promise<void>;
  grantScannerRole(userId: string): Promise<void>;
  revokeScannerRole(userId: string): Promise<void>;
  assignReviewer(
    applicationId: string,
    request: AssignReviewerRequest,
  ): Promise<ReviewerAssignmentResult>;
  listReviewerApplications(): Promise<ReviewerApplicationListResponse>;
  getReviewerApplication(applicationId: string): Promise<ReviewerApplication>;
  saveReviewDraft(
    applicationId: string,
    request: SaveReviewDraftRequest,
  ): Promise<ReviewerApplication>;
  submitReview(
    applicationId: string,
    lockVersion: number,
  ): Promise<ReviewerApplication>;
};

export function createApiClient(options: ApiClientOptions = {}): ApiClient {
  const configuredBaseUrl =
    options.baseUrl ??
    process.env.NEXT_PUBLIC_API_BASE_URL ??
    DEFAULT_API_BASE_URL;
  const baseUrl = configuredBaseUrl.replace(/\/+$/, "");

  async function request<TResponse>(
    path: ApiPath,
    init: RequestInit = {},
  ): Promise<TResponse> {
    const headers = new Headers(init.headers);
    headers.set("Accept", "application/json");

    if (init.body && !(init.body instanceof FormData)) {
      headers.set("Content-Type", "application/json");
    }

    const token = await options.getToken?.();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }

    const response = await fetch(`${baseUrl}${path}`, {
      ...init,
      headers,
    });

    if (response.status === 204) {
      return undefined as TResponse;
    }

    const body = (await response.json()) as TResponse | ApiErrorBody;
    if (!response.ok) {
      throw new ApiError(response.status, body as ApiErrorBody);
    }

    return body as TResponse;
  }

  function filenameFromContentDisposition(value: string | null): string | null {
    if (!value) {
      return null;
    }

    const extendedMatch = value.match(/filename\*=UTF-8''([^;]+)/i);
    const plainMatch = value.match(/filename="?([^";]+)"?/i);
    const candidate = extendedMatch?.[1] ?? plainMatch?.[1];
    if (!candidate) {
      return null;
    }

    let decoded = candidate;
    try {
      decoded = decodeURIComponent(candidate);
    } catch {
      // Use the header value after filename sanitization when it is not URI-encoded.
    }

    const safeFilename = decoded.replace(/[^A-Za-z0-9._-]/g, "_").slice(0, 128);
    return safeFilename || null;
  }

  async function downloadCsv(path: ApiPath): Promise<OrganizerCsvDownload> {
    const headers = new Headers();
    headers.set("Accept", "text/csv");

    const token = await options.getToken?.();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }

    const response = await fetch(`${baseUrl}${path}`, { headers });
    if (!response.ok) {
      let body: ApiErrorBody = {};
      try {
        body = (await response.json()) as ApiErrorBody;
      } catch {
        // CSV download errors are expected to use the JSON API error envelope.
      }
      throw new ApiError(response.status, body);
    }

    return {
      blob: await response.blob(),
      filename: filenameFromContentDisposition(
        response.headers.get("Content-Disposition"),
      ),
    };
  }

  async function downloadPdf(path: ApiPath): Promise<Blob> {
    const headers = new Headers({ Accept: "application/pdf" });
    const token = await options.getToken?.();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
    const response = await fetch(`${baseUrl}${path}`, { headers });
    if (!response.ok) {
      let body: ApiErrorBody = {};
      try {
        body = (await response.json()) as ApiErrorBody;
      } catch {
        // PDF endpoint errors use the JSON error envelope.
      }
      throw new ApiError(response.status, body);
    }
    return response.blob();
  }

  function organizerExportPath(
    filename: "attendance.csv" | "reconciliation.csv",
    filters: OrganizerExportFilters = {},
  ): ApiPath {
    const searchParameters = new URLSearchParams();
    if (filters.checkpointId) {
      searchParameters.set("checkpointId", filters.checkpointId);
    }
    const query = searchParameters.toString();
    return `/v1/admin/exports/${filename}${query ? `?${query}` : ""}` as ApiPath;
  }

  return {
    request,
    getCurrentUser: () => request<CurrentUser>("/v1/me"),
    getCurrentApplicationForm: () =>
      request<CurrentApplicationForm>("/v1/application-forms/current"),
    createApplication: () =>
      request<ApplicantApplication>("/v1/applications", { method: "POST" }),
    getMyApplications: () =>
      request<MyApplicationsResponse>("/v1/applications/mine"),
    saveApplicationDraft: (applicationId, draft) =>
      request<ApplicantApplication>(`/v1/applications/${applicationId}/draft`, {
        method: "PUT",
        body: JSON.stringify(draft),
      }),
    submitApplication: (applicationId, submission) =>
      request<ApplicantApplication>(`/v1/applications/${applicationId}/submit`, {
        method: "POST",
        body: JSON.stringify(submission),
      }),
    getApplicationResume: (applicationId) =>
      request<ApplicationResume>(`/v1/applications/${applicationId}/resume`),
    uploadApplicationResume: async (applicationId, file) => {
      const headers = new Headers({
        Accept: "application/json",
        "Content-Type": "application/pdf",
        "X-File-Name": file.name.replace(/[^A-Za-z0-9._-]/g, "_").slice(0, 128),
      });
      const token = await options.getToken?.();
      if (token) {
        headers.set("Authorization", `Bearer ${token}`);
      }
      const response = await fetch(
        `${baseUrl}/v1/applications/${applicationId}/resume`,
        { method: "PUT", headers, body: file },
      );
      const body = (await response.json()) as ApplicationResume | ApiErrorBody;
      if (!response.ok) {
        throw new ApiError(response.status, body as ApiErrorBody);
      }
      return body as ApplicationResume;
    },
    downloadAdminApplicationResume: (applicationId) =>
      downloadPdf(`/v1/admin/applications/${applicationId}/resume`),
    listOrganizerApplications: (filters = {}) => {
      const searchParameters = new URLSearchParams();
      if (filters.status) {
        searchParameters.set("status", filters.status);
      }
      if (filters.q) {
        searchParameters.set("q", filters.q);
      }
      const query = searchParameters.toString();
      return request<OrganizerApplicationListResponse>(
        `/v1/admin/applications${query ? `?${query}` : ""}` as ApiPath,
      );
    },
    getOrganizerApplication: (applicationId) =>
      request<OrganizerApplication>(`/v1/admin/applications/${applicationId}`),
    getAttendeePass: () =>
      request<AuthenticatedAttendeePass>("/v1/attendee/pass"),
    issueAttendeePass: (attendeeId) =>
      request<PassIssuance>(`/v1/admin/attendees/${attendeeId}/passes`, {
        method: "POST",
      }),
    revokeAttendeePass: (passId) =>
      request<AttendeePass>(`/v1/admin/passes/${passId}/revoke`, {
        method: "POST",
      }),
    reissueAttendeePass: (passId) =>
      request<PassIssuance>(`/v1/admin/passes/${passId}/reissue`, {
        method: "POST",
      }),
    listScannerCheckpoints: (cursor) => {
      const query = cursor
        ? `?${new URLSearchParams({ cursor }).toString()}`
        : "";
      return request<ScannerCheckpointListResponse>(
        `/v1/checkpoints${query}` as ApiPath,
      );
    },
    lookupScannerPass: (lookup) =>
      request<ScannerLookupResponse>("/v1/scans/lookup", {
        method: "POST",
        body: JSON.stringify(lookup),
      }),
    redeemScannerPass: (redemption) =>
      request<ScannerRedemptionResponse>("/v1/redemptions", {
        method: "POST",
        body: JSON.stringify(redemption),
      }),
    listOrganizerActivities: () =>
      request<OrganizerActivityListResponse>("/v1/admin/activities"),
    createOrganizerActivity: (activity) =>
      request<OrganizerActivity>("/v1/admin/activities", {
        method: "POST",
        body: JSON.stringify(activity),
      }),
    updateOrganizerActivity: (activityId, activity) =>
      request<OrganizerActivity>(`/v1/admin/activities/${activityId}`, {
        method: "PATCH",
        body: JSON.stringify(activity),
      }),
    deleteOrganizerActivity: (activityId) =>
      request<void>(`/v1/admin/activities/${activityId}`, {
        method: "DELETE",
      }),
    listOrganizerCheckpoints: () =>
      request<OrganizerCheckpointListResponse>("/v1/admin/checkpoints"),
    createOrganizerCheckpoint: (checkpoint) =>
      request<OrganizerCheckpoint>("/v1/admin/checkpoints", {
        method: "POST",
        body: JSON.stringify(checkpoint),
      }),
    updateOrganizerCheckpoint: (checkpointId, checkpoint) =>
      request<OrganizerCheckpoint>(`/v1/admin/checkpoints/${checkpointId}`, {
        method: "PATCH",
        body: JSON.stringify(checkpoint),
      }),
    deleteOrganizerCheckpoint: (checkpointId) =>
      request<void>(`/v1/admin/checkpoints/${checkpointId}`, {
        method: "DELETE",
      }),
    getOrganizerAttendeeEntitlement: (attendeeId, checkpointId) =>
      request<OrganizerAttendeeEntitlementResponse>(
        `/v1/admin/attendees/${attendeeId}/entitlements/${checkpointId}`,
      ),
    updateOrganizerAttendeeEntitlement: (attendeeId, checkpointId, entitlement) =>
      request<OrganizerEntitlement>(
        `/v1/admin/attendees/${attendeeId}/entitlements/${checkpointId}`,
        {
          method: "PUT",
          body: JSON.stringify(entitlement),
        },
      ),
    deleteOrganizerAttendeeEntitlement: (attendeeId, checkpointId) =>
      request<void>(
        `/v1/admin/attendees/${attendeeId}/entitlements/${checkpointId}`,
        {
          method: "DELETE",
        },
      ),
    listOrganizerRedemptionCounts: () =>
      request<OrganizerRedemptionCountsResponse>("/v1/admin/redemptions/counts"),
    listOrganizerRedemptions: () =>
      request<OrganizerRedemptionListResponse>("/v1/admin/redemptions"),
    downloadOrganizerAttendanceCsv: (filters) =>
      downloadCsv(organizerExportPath("attendance.csv", filters)),
    downloadOrganizerReconciliationCsv: (filters) =>
      downloadCsv(organizerExportPath("reconciliation.csv", filters)),
    recordOrganizerDecision: (applicationId, decision) =>
      request<OrganizerDecision>(`/v1/admin/applications/${applicationId}/decisions`, {
        method: "POST",
        body: JSON.stringify(decision),
      }),
    releaseOrganizerDecision: (decisionId) =>
      request<OrganizerDecision>(`/v1/admin/decisions/${decisionId}/release`, {
        method: "POST",
      }),
    grantReviewerRole: (userId) =>
      request<void>(`/v1/admin/users/${userId}/roles/reviewer`, {
        method: "PUT",
      }),
    grantScannerRole: (userId) =>
      request<void>(`/v1/admin/users/${userId}/roles/scanner`, {
        method: "PUT",
      }),
    revokeScannerRole: (userId) =>
      request<void>(`/v1/admin/users/${userId}/roles/scanner`, {
        method: "DELETE",
      }),
    assignReviewer: (applicationId, assignment) =>
      request<ReviewerAssignmentResult>(
        `/v1/admin/applications/${applicationId}/assignments`,
        {
          method: "POST",
          body: JSON.stringify(assignment),
        },
      ),
    listReviewerApplications: () =>
      request<ReviewerApplicationListResponse>("/v1/reviewer/assignments"),
    getApplicationDecision: (applicationId) =>
      request<ApplicantReleasedDecision>(`/v1/applications/${applicationId}/decision`),
    getReviewerApplication: (applicationId) =>
      request<ReviewerApplication>(`/v1/reviewer/applications/${applicationId}`),
    saveReviewDraft: (applicationId, review) =>
      request<ReviewerApplication>(
        `/v1/reviewer/applications/${applicationId}/review`,
        {
          method: "PUT",
          body: JSON.stringify(review),
        },
      ),
    submitReview: (applicationId, lockVersion) =>
      request<ReviewerApplication>(
        `/v1/reviewer/applications/${applicationId}/review/submit`,
        {
          method: "POST",
          body: JSON.stringify({ lockVersion }),
        },
      ),
  };
}
