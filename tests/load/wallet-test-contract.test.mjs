import test from "node:test";
import assert from "node:assert/strict";
import { generateKeyPairSync, privateDecrypt, createDecipheriv } from "node:crypto";
import { assertWalletTestTarget, encryptDelivery, inspectSaveURL, inspectDisabledExport, STAGING_ORIGIN, TEST_CLASS } from "./wallet-test-contract.mjs";
const db="postgresql://postgres.ovzrhurmiwqthfgycamx:fixture@aws-0-ca-central-1.pooler.supabase.com:5432/postgres";
const settings={GOOGLE_WALLET_CLASS_ID:TEST_CLASS,GOOGLE_WALLET_ENABLED:"false"};
test("restoration verifies disabled flag and records edge 504 as a failed 503 contract",()=>{
  const pass={googleWalletAvailable:false};
  assert.equal(inspectDisabledExport(pass,503,{}).disabledExport503ContractPassed,true);
  assert.deepEqual(inspectDisabledExport(pass,504,{}),{disabledExportHTTPStatus:504,disabledExport503ContractPassed:false});
  for(const [p,status,body] of [[{googleWalletAvailable:true},504,{}],[pass,200,{}],[pass,401,{}],[pass,504,{saveUrl:"unexpected"}]]) assert.throws(()=>inspectDisabledExport(p,status,body));
});
test("reject production, other database, real class, and enabled baseline",()=>{
  assert.doesNotThrow(()=>assertWalletTestTarget(STAGING_ORIGIN,db,settings));
  for(const [origin,url,config] of [["https://api.hackatlantic.ca",db,settings],[STAGING_ORIGIN,db.replace("ovzrhurmiwqthfgycamx","oizbfvfcownivwsrzlml"),settings],[STAGING_ORIGIN,db,{...settings,GOOGLE_WALLET_CLASS_ID:"3388000000023208272.hackatlantic_2026"}],[STAGING_ORIGIN,db,{...settings,GOOGLE_WALLET_ENABLED:"true"}]]) assert.throws(()=>assertWalletTestTarget(origin,url,config));
});
test("encrypt delivery; ciphertext cannot be read without local private key",()=>{
  const {publicKey,privateKey}=generateKeyPairSync("rsa",{modulusLength:3072});
  const value={saveUrl:"synthetic-secret",qrToken:"synthetic-qr"};
  const e=encryptDelivery(value,publicKey.export({type:"spki",format:"pem"}));
  assert.ok(!JSON.stringify(e).includes(value.saveUrl));
  const key=privateDecrypt({key:privateKey,oaepHash:"sha256"},Buffer.from(e.key,"base64"));
  const decipher=createDecipheriv("aes-256-gcm",key,Buffer.from(e.iv,"base64"));
  decipher.setAuthTag(Buffer.from(e.tag,"base64"));
  assert.deepEqual(JSON.parse(Buffer.concat([decipher.update(Buffer.from(e.data,"base64")),decipher.final()])),value);
});
test("Wallet and web pass must carry identical credential and TEST class",()=>{
  const pass={id:"12345678-1234-1234-1234-123456789012",qrToken:"synthetic-qr"};
  const object={id:"3388000000023208272.pass_"+pass.id.replaceAll("-",""),classId:TEST_CLASS,barcode:{value:pass.qrToken}};
  const link=(o)=>"https://pay.google.com/gp/v/save/header."+Buffer.from(JSON.stringify({exp:Math.floor(Date.now()/1000)+600,payload:{eventTicketObjects:[o]}})).toString("base64url")+".signature";
  assert.equal(inspectSaveURL(link(object),pass),object.id);
  assert.throws(()=>inspectSaveURL(link({...object,barcode:{value:"other"}}),pass));
  assert.throws(()=>inspectSaveURL(link({...object,classId:"production"}),pass));
  assert.throws(()=>inspectSaveURL(link(object).replace("pay.google.com","evil.example"),pass));
});
