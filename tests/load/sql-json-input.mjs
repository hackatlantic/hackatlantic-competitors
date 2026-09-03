// Send large fixture expectations in psql's stdin SQL, not a process argument.
// Base64 has no SQL delimiters; neither fixture values nor credentials are logged.
export function jsonbInput(value) {
  const encoded = Buffer.from(JSON.stringify(value), "utf8").toString("base64");
  return `convert_from(decode('${encoded}', 'base64'), 'UTF8')::jsonb`;
}
