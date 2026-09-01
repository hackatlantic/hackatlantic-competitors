const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-8[0-9a-f]{3}-[0-9a-f]{12}$/;

function decimalSegment(value, width, name) {
  const normalized = String(value);
  if (!/^\d+$/.test(normalized)) throw new Error(`${name} must contain only decimal digits`);
  if (normalized.length > width) throw new Error(`${name} exceeds its ${width}-digit UUID allocation`);
  return normalized.padStart(width, "0");
}

/**
 * Returns an RFC 4122-shaped UUID that is stable for one k6 operation while
 * remaining unique across GitHub workflow runs. Staging deliberately retains
 * redemption requests for auditability, so VU/iteration-only identifiers are
 * not sufficient: a later benchmark would otherwise replay an old key against
 * a different pass and receive HTTP 409.
 */
export function loadTestOperationID(runID, virtualUserID, iteration) {
  const runDigits = String(runID).replace(/\D/g, "");
  if (!runDigits) throw new Error("runID must contain at least one decimal digit");

  // GitHub run IDs are monotonic and currently fit comfortably inside twelve
  // digits. Keeping the least-significant twelve also supports Date.now()-based
  // local fixtures for hundreds of years without truncation.
  const run = runDigits.padStart(12, "0").slice(-12);
  const vu = decimalSegment(virtualUserID, 3, "virtualUserID");
  const operation = decimalSegment(iteration, 12, "iteration");
  const id = `${run.slice(0, 8)}-${run.slice(8)}-4000-8${vu}-${operation}`;

  if (!uuidPattern.test(id)) throw new Error("generated load-test operation ID is not a valid UUID");
  return id;
}
