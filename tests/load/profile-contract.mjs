// Pure workload definitions shared by k6, fixture preparation, and Node tests.
export const STAGING_API = "https://hackatlantic-api-staging-5c4l8.ondigitalocean.app";
export const RESUME_BYTES = 512 * 1024;

export function assertStagingTarget(url) {
  if (url !== STAGING_API) throw new Error("Load tests are restricted to the exact staging API origin");
}

export function tokenAt(identity, now = Math.floor(Date.now() / 1000)) {
  if (!identity.tokenWindows) return identity.token; // Existing short release fixtures.
  const window = [...identity.tokenWindows].reverse().find((item) => item.iat <= now && item.exp > now + 30);
  if (!window) throw new Error("No valid short-lived synthetic token window remains");
  return window.token;
}

// Paired jitter is reproducible and totals seven seconds per pair. It bounds
// the ten-minute fixture capacity without allocating thousands of unused passes.
export function scannerPause(vu, iteration) {
  const first = 2 + (((vu * 977 + Math.floor(iteration / 2) * 619) % 3001) / 1000);
  return iteration % 2 === 0 ? first : 7 - first;
}

export function scannerScenario(profile, vus, iterations, env = {}) {
  if (profile === "repeatability") return { executor: "constant-vus", vus: 20, duration: "10m", gracefulStop: "30s" };
  if (profile === "spike") return {
    executor: "constant-arrival-rate", rate: Number(env.K6_SCANNER_RATE ?? 5), timeUnit: "1s",
    duration: env.K6_SCANNER_DURATION ?? "20s", preAllocatedVUs: Math.min(vus, 20), maxVUs: vus,
  };
  if (!["release", "contention"].includes(profile)) throw new Error("Unknown scanner profile");
  return { executor: "shared-iterations", vus, iterations, maxDuration: env.K6_SCANNER_MAX_DURATION ?? (profile === "release" ? "10m" : "2m") };
}

export function applicantScenario(profile, vus, count, env = {}) {
  if (profile === "deadline") {
    if (count !== 250) throw new Error("The deadline profile needs exactly 250 prepared applicants");
    return { executor: "constant-arrival-rate", rate: 25, timeUnit: "1m", duration: "10m", preAllocatedVUs: 10, maxVUs: 25, gracefulStop: "30s" };
  }
  if (profile === "stress") return { executor: "per-vu-iterations", vus, iterations: 1, maxDuration: env.K6_APPLICANT_MAX_DURATION ?? "2m" };
  if (profile !== "sustained") throw new Error("Unknown applicant profile");
  return { executor: "shared-iterations", vus, iterations: count, maxDuration: env.K6_APPLICANT_MAX_DURATION ?? "15m", gracefulStop: "30s" };
}

export function applicantAnswers(form, email, sequence, revision = 1) {
  const answers = Object.fromEntries(form.questions.map((question) => {
    const description = question.key + " " + question.label;
    if (question.type === "boolean") return [question.key, true];
    if (question.type === "number") return [question.key, sequence + 1];
    if (question.options?.length) return [question.key, question.options[0]];
    if (/email/i.test(description)) return [question.key, email];
    if (/school/i.test(description)) return [question.key, "Synthetic Atlantic University"];
    return [question.key, `Synthetic response ${sequence + 1} revision ${revision}`];
  }));
  return Object.fromEntries(Object.entries(answers).filter(([key]) => {
    const question = form.questions.find((candidate) => candidate.key === key);
    return !question.showWhen || answers[question.showWhen.key] === question.showWhen.equals;
  }));
}

export function shouldUpload(form, index, profile) {
  return form.resumeRequired || (profile !== "deadline" && index % 2 === 0);
}

// An ASCII-only, deterministic one-page PDF, with exact size and valid xref.
export function fixedResume() {
  let pdf = "%PDF-1.4\n";
  const offsets = [0];
  const objects = [
    "<< /Type /Catalog /Pages 2 0 R >>",
    "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
  ];
  objects.forEach((body, i) => { offsets.push(pdf.length); pdf += `${i + 1} 0 obj\n${body}\nendobj\n`; });
  const xref = "xref\n0 4\n0000000000 65535 f \n" + offsets.slice(1).map((offset) => `${String(offset).padStart(10, "0")} 00000 n \n`).join("");
  const tail = (offset) => xref + `trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n${String(offset).padStart(10, "0")}\n%%EOF\n`;
  const xrefOffset = RESUME_BYTES - tail(0).length;
  // Pad before xref so its pointer and EOF remain adjacent for real PDF readers.
  return pdf + " ".repeat(xrefOffset - pdf.length) + tail(xrefOffset);
}
