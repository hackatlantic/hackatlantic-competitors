import { createPublicKey, publicEncrypt, randomBytes, createCipheriv } from "node:crypto";

export const TEST_CLASS = "3388000000023208272.hackatlantic_2026_test";
export const STAGING_ORIGIN = "https://hackatlantic-api-staging-5c4l8.ondigitalocean.app";

export function assertWalletTestTarget(origin, databaseURL, settings) {
  const db = new URL(databaseURL);
  if (origin !== STAGING_ORIGIN || db.hostname !== "aws-0-ca-central-1.pooler.supabase.com" ||
      decodeURIComponent(db.username) !== "postgres.ovzrhurmiwqthfgycamx" ||
      settings.GOOGLE_WALLET_CLASS_ID !== TEST_CLASS || settings.GOOGLE_WALLET_ENABLED !== "false") {
    throw new Error("Wallet rehearsal requires the isolated staging database, TEST class and disabled baseline");
  }
}

export function encryptDelivery(value, pem) {
  const publicKey = createPublicKey(pem);
  if (publicKey.asymmetricKeyType !== "rsa" || publicKey.asymmetricKeyDetails.modulusLength < 3072) {
    throw new Error("Delivery requires an RSA public key of at least 3072 bits");
  }
  const key = randomBytes(32), iv = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", key, iv);
  const encrypted = Buffer.concat([cipher.update(JSON.stringify(value), "utf8"), cipher.final()]);
  return { version: 1, algorithm: "RSA-OAEP-SHA256+A256GCM",
    key: publicEncrypt({key: publicKey, oaepHash: "sha256"}, key).toString("base64"),
    iv: iv.toString("base64"), tag: cipher.getAuthTag().toString("base64"), data: encrypted.toString("base64") };
}

export function inspectSaveURL(saveURL, pass) {
  const url = new URL(saveURL);
  if (url.origin !== "https://pay.google.com" || url.username || url.password || url.search || url.hash ||
      !url.pathname.startsWith("/gp/v/save/")) throw new Error("Unexpected Wallet save destination");
  const jwt = url.pathname.slice("/gp/v/save/".length);
  const claims = JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString("utf8"));
  const object = claims.payload?.eventTicketObjects?.[0];
  if (object?.classId !== TEST_CLASS || object.barcode?.value !== pass.qrToken ||
      object.id !== "3388000000023208272.pass_" + pass.id.replaceAll("-", "") || !Number.isFinite(claims.exp) || claims.exp * 1000 <= Date.now()) {
    throw new Error("Wallet payload does not match the active staging-issued pass");
  }
  return object.id;
}
