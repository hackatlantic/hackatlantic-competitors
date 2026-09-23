// Bounded, synthetic-only device rehearsal. Never prints bearer values or cloud credentials.
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { randomUUID } from "node:crypto";
import { assertWalletTestTarget, encryptDelivery, inspectSaveURL, STAGING_ORIGIN } from "./wallet-test-contract.mjs";

const root = "infra/environments/staging";
const vars = JSON.parse(process.env.TF_VARS_JSON);
const baseline = JSON.parse(process.env.API_WALLET_ENV_JSON);
process.env.API_BASE_URL = STAGING_ORIGIN;
process.env.DATABASE_URL = `postgresql://postgres.ovzrhurmiwqthfgycamx:${encodeURIComponent(vars.supabase_database_password)}@aws-0-ca-central-1.pooler.supabase.com:5432/postgres?sslmode=require`;
process.env.PGCONNECT_TIMEOUT = "15";
process.env.PGOPTIONS = "-c statement_timeout=30000 -c lock_timeout=10000";
assertWalletTestTarget(process.env.API_BASE_URL, process.env.DATABASE_URL, baseline);
if (["exercise", "verify-restored"].includes(process.argv[2])) process.env.LOAD_TEST_AUTH_SECRET = output("load_test_auth_secret");
const { psql, createIdentity, sessionTokens, seedAcceptedAttendees } = await import("./staging-fixture.mjs");
const contextPath = ".tmp/wallet-rehearsal-context.json";
const evidencePath = ".tmp/wallet-rehearsal-evidence.json";
mkdirSync(".tmp", {recursive:true});
function save(path, value) { writeFileSync(path, JSON.stringify(value), {mode:0o600}); }
function load() { return JSON.parse(readFileSync(contextPath, "utf8")); }
function output(name) { return execFileSync("terraform", [`-chdir=${root}`, "output", "-raw", name], {encoding:"utf8", timeout:60000, stdio:["ignore","pipe","pipe"]}).trim(); }
function requireTest(condition, message) { if (!condition) throw new Error(message); }

async function request(path, identity, {method="GET", body, expected=200}={}) {
  const headers = {"Content-Type":"application/json"};
  if (identity) headers.Authorization = "Bearer " + (await sessionTokens([identity]))[0];
  const response = await fetch(STAGING_ORIGIN + path, {method, headers, redirect:"error", signal:AbortSignal.timeout(30000), body:body===undefined?undefined:JSON.stringify(body)});
  const json = await response.json().catch(()=>({}));
  requireTest(response.status === expected, `Staging request failed expected status (${method}, ${expected}, got ${response.status})`);
  if (path.includes("google-wallet")) requireTest(response.headers.get("cache-control")==="no-store", "Wallet response must not be cached");
  return json;
}

async function preflight() {
  // Validate the recipient key before any infrastructure or fixture mutation.
  encryptDelivery({preflight:true}, process.env.WALLET_DELIVERY_PUBLIC_KEY);
  requireTest(output("supabase_project_ref")==="ovzrhurmiwqthfgycamx", "Wrong Terraform database target");
  requireTest(output("api_default_ingress")===STAGING_ORIGIN, "Wrong Terraform API target");
  const appID = output("api_app_id");
  const result = await fetch(`https://api.digitalocean.com/v2/apps/${appID}`, {headers:{Authorization:`Bearer ${process.env.DIGITALOCEAN_TOKEN}`}, signal:AbortSignal.timeout(30000)});
  requireTest(result.ok, "Cannot read staging app");
  const {app} = await result.json();
  requireTest(app.spec?.name === "hackatlantic-api-staging" && app.active_deployment?.phase === "ACTIVE" && !app.in_progress_deployment, "Staging must be stable before rehearsal");
  const digest = app.active_deployment.spec.services.find(s=>s.name==="api")?.image?.digest;
  requireTest(/^sha256:[a-f0-9]{64}$/.test(digest), "Staging image must be pinned");
  const version = await request("/versionz");
  requireTest(version.environment==="staging", "Wrong live API environment");
  const form = JSON.parse(psql(`SELECT json_build_object('cycleId',c.id,'slug',c.slug,'id',f.id)
    FROM ats.application_cycles c JOIN LATERAL (SELECT id FROM ats.application_forms WHERE cycle_id=c.id AND published_at IS NOT NULL ORDER BY version DESC LIMIT 1) f ON true WHERE c.active;`));
  requireTest(/^[A-Za-z0-9_-]{1,80}$/.test(form.slug), "Missing valid active staging form");
  const runID = `wallet_${process.env.GITHUB_RUN_ID}_${process.env.GITHUB_RUN_ATTEMPT}`;
  requireTest(/^wallet_\d+_\d+$/.test(runID), "Expected GitHub rehearsal identity");
  save(contextPath, {runID, form, digest, gitSha:version.gitSha, admin:createIdentity(runID,"admin"), scanner:createIdentity(runID,"scanner"), outsider:createIdentity(runID,"outsider")});
  save(".tmp/wallet-disabled.tfvars.json", {api_wallet_env:baseline, api_image_digest:digest});
  save(".tmp/wallet-enabled.tfvars.json", {api_wallet_env:{...baseline, GOOGLE_WALLET_ENABLED:"true", GOOGLE_WALLET_CYCLE_SLUG:form.slug}, api_image_digest:digest});
  console.log("Preflight passed: isolated staging, TEST class, pinned current image; baseline disabled settings retained for restoration.");
}

async function exercise() {
  process.env.LOAD_TEST_AUTH_SECRET = output("load_test_auth_secret");
  const ctx = load();
  requireTest((await request("/versionz")).gitSha===ctx.gitSha, "Image changed during rehearsal");
  psql("INSERT INTO ats.admin_email_allowlist(normalized_email) VALUES (:'email') ON CONFLICT DO NOTHING", {email:ctx.admin.email});
  await request("/v1/me", ctx.admin);
  const scanner = await request("/v1/me", ctx.scanner);
  await request(`/v1/admin/users/${scanner.id}/roles/scanner`, ctx.admin, {method:"PUT",body:{},expected:204});
  await request("/v1/me", ctx.outsider);
  const [attendee] = seedAcceptedAttendees(ctx.form,ctx.runID,1,ctx.admin.userId);
  ctx.attendee = attendee;
  save(contextPath, ctx);
  psql("UPDATE ats.attendees SET display_name='TEST - STAGING ONLY' WHERE id=:'id'::uuid", {id:attendee.attendee_id});
  const owner = {userId:attendee.clerk_user_id};
  const walletPath = "/v1/attendee/pass/google-wallet";
  const post = {method:"POST"};
  await request(walletPath, null, {...post, expected:401});
  // All real accounts also have applicant access; scanner privileges alone do not grant a pass.
  await request(walletPath, ctx.scanner, {...post, expected:404});
  await request(walletPath, owner, {...post, expected:404}); // accepted but pass not released
  const first = await request(`/v1/admin/attendees/${attendee.attendee_id}/passes`,ctx.admin,{...post,expected:201});
  ctx.firstPassID=first.id;
  save(contextPath,ctx);
  let rsvp = await request(`/v1/applications/${attendee.application_id}/rsvp`,owner);
  rsvp = await request(`/v1/applications/${attendee.application_id}/rsvp`,owner,{method:"PUT",body:{decisionId:rsvp.decisionId,lockVersion:rsvp.lockVersion,status:"declined"}});
  await request(walletPath,owner,{...post,expected:404});
  await request(`/v1/applications/${attendee.application_id}/rsvp`,owner,{method:"PUT",body:{decisionId:rsvp.decisionId,lockVersion:rsvp.lockVersion,status:"confirmed"}});
  await request(walletPath,ctx.outsider,{...post,expected:404});
  const firstWeb = await request("/v1/attendee/pass",owner);
  requireTest(firstWeb.googleWalletAvailable===true, "Wallet flag did not activate");
  const firstSave = await request(walletPath,owner,post);
  const firstObjectID=inspectSaveURL(firstSave.saveUrl,firstWeb);
  const duplicateSave = await request(walletPath,owner,post);
  requireTest(inspectSaveURL(duplicateSave.saveUrl,firstWeb)===firstObjectID, "Repeated save changed the object");
  async function checkpoint(suffix) {
    return request("/v1/admin/checkpoints",ctx.admin,{...post,body:{cycleId:ctx.form.cycleId,activityId:null,slug:ctx.runID+"-"+suffix,name:"TEST Wallet "+suffix,opensAt:null,closesAt:null,defaultAllowed:true,defaultMaxRedemptions:1,active:true},expected:201});
  }
  const entry=await checkpoint("entry"), meal=await checkpoint("replacement"), device=await checkpoint("device");
  ctx.checkpoints=[entry.id,meal.id,device.id]; save(contextPath,ctx);
  async function redeem(qrToken,cp,idempotencyKey=randomUUID()) {
    return request("/v1/redemptions",ctx.scanner,{...post,body:{qrToken,checkpointId:cp.id,idempotencyKey}});
  }
  const lookup=await request("/v1/scans/lookup",ctx.scanner,{...post,body:{qrToken:firstWeb.qrToken}});
  requireTest(lookup.pass.status==="active" && lookup.attendee.displayName==="TEST - STAGING ONLY", "Scanner resolved wrong attendee or state");
  const key=randomUUID(), checkedIn=await redeem(firstWeb.qrToken,entry,key);
  requireTest(checkedIn.outcome==="redeemed", "First check-in failed");
  requireTest((await redeem(firstWeb.qrToken,entry,key)).redemptionId===checkedIn.redemptionId,"Idempotent replay changed ledger");
  requireTest((await redeem(firstWeb.qrToken,entry)).outcome==="already_exhausted","Duplicate check-in was not blocked");
  await request(`/v1/admin/passes/${first.id}/revoke`,ctx.admin,post);
  requireTest((await redeem(firstWeb.qrToken,meal)).outcome==="revoked_pass","Revoked pass still redeems");
  await request(walletPath,owner,{...post,expected:404});
  const second=await request(`/v1/admin/attendees/${attendee.attendee_id}/passes`,ctx.admin,{...post,expected:201});
  const replacement=await request(`/v1/admin/passes/${second.id}/reissue`,ctx.admin,{...post,expected:201});
  requireTest((await redeem(second.qrToken,meal)).outcome==="invalid_pass","Replaced pass still redeems");
  requireTest((await redeem(replacement.qrToken,entry)).outcome==="already_exhausted","Replacement bypassed attendee redemption limit");
  requireTest((await redeem(replacement.qrToken,meal)).outcome==="redeemed","Replacement failed at unused checkpoint");
  const web=await request("/v1/attendee/pass",owner);
  const saved=await request(walletPath,owner,post);
  inspectSaveURL(saved.saveUrl,web);
  const ledger=JSON.parse(psql(`SELECT json_build_object('redemptions',count(*),'checkpoints',count(DISTINCT checkpoint_id),'maxOrdinal',max(ordinal)) FROM ats.redemptions WHERE checkpoint_id=ANY(ARRAY[:'entry'::uuid,:'meal'::uuid,:'device'::uuid])`,{entry:entry.id,meal:meal.id,device:device.id}));
  requireTest(ledger.redemptions===2 && ledger.checkpoints===2 && ledger.maxOrdinal===1,"Unexpected redemption ledger");
  save(".tmp/wallet-device-delivery.encrypted.json",encryptDelivery({runID:ctx.runID,saveUrl:saved.saveUrl,webPass:web,revokedWebPass:firstWeb,checkpointId:device.id,apiBaseURL:STAGING_ORIGIN},process.env.WALLET_DELIVERY_PUBLIC_KEY));
  save(evidencePath,{runID:ctx.runID,gitSha:ctx.gitSha,automatedAPIResults:"passed",eligibilityChecks:["unauthenticated","scanner role","unreleased pass","declined RSVP","different owner"],walletQRMatchesWeb:true,duplicateSaveStable:true,firstScan:"redeemed",repeatScan:"already_exhausted",revokedScan:"revoked_pass",replacedScan:"invalid_pass",replacementPreservesLimits:true,replacementUnusedCheckpoint:"redeemed",ledger,physicalAndroidScan:"pending",productionChanged:false});
  console.log("Wallet-enabled staging API checks passed; encrypted TEST-pass delivery prepared. Physical Android capture remains pending.");
}

function cleanup() {
  if (!existsSync(contextPath)) return;
  const ctx=load();
  psql("DELETE FROM ats.user_roles WHERE role='scanner' AND user_id IN (SELECT id FROM ats.users WHERE clerk_user_id=:'subject')",{subject:ctx.scanner.userId});
  psql("DELETE FROM ats.admin_email_allowlist WHERE normalized_email=:'email'",{email:ctx.admin.email});
  console.log("Removed this rehearsal's temporary staff privileges; synthetic ledger retained in isolated staging.");
}

async function verifyRestored() {
  process.env.LOAD_TEST_AUTH_SECRET=output("load_test_auth_secret");
  const ctx=load();
  requireTest((await request("/versionz")).gitSha===ctx.gitSha,"Restoration changed API image");
  if (ctx.attendee) await request("/v1/attendee/pass/google-wallet",{userId:ctx.attendee.clerk_user_id},{method:"POST",expected:503});
  if (existsSync(evidencePath)) {const e=JSON.parse(readFileSync(evidencePath));e.walletDisabledAfterTest=true;e.staffCleanupPassed=true;save(evidencePath,e);}
  console.log("Verified Wallet is disabled again and the original staging API image is unchanged.");
}

try {
  const actions={preflight,exercise,cleanup,"verify-restored":verifyRestored};
  requireTest(!!actions[process.argv[2]],"Unknown rehearsal action");
  await actions[process.argv[2]]();
} catch (error) {
  // No raw provider/DB response, save link, or token is emitted on failure.
  console.error(error instanceof Error ? error.message.split("\n")[0].slice(0,180) : "Wallet rehearsal failed");
  process.exitCode=1;
}
