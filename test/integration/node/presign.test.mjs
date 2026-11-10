import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { setTimeout as sleep } from "node:timers/promises";
import { PutObjectCommand, GetObjectCommand } from "@aws-sdk/client-s3";
import { getSignedUrl } from "@aws-sdk/s3-request-presigner";
import { client, makeClient, endpoint, uniqueBucket, createBucket, emptyAndDelete, bodyBuffer, closeClients } from "./lib.mjs";

const Bucket = uniqueBucket("presign");
before(() => createBucket(Bucket));
after(async () => {
  await emptyAndDelete(Bucket);
  closeClients();
});

test("presigned GET via fetch", async () => {
  await client.send(new PutObjectCommand({ Bucket, Key: "get me", Body: "presigned body" }));
  const url = await getSignedUrl(client, new GetObjectCommand({ Bucket, Key: "get me" }), { expiresIn: 300 });
  const res = await fetch(url);
  assert.equal(res.status, 200);
  assert.equal(await res.text(), "presigned body");
});

test("presigned PUT via fetch (default checksum calculation)", async () => {
  const url = await getSignedUrl(client, new PutObjectCommand({ Bucket, Key: "put-default" }), { expiresIn: 300 });
  const res = await fetch(url, { method: "PUT", body: "" });
  assert.equal(res.status, 200, await res.text());
});

test("presigned PUT with requestChecksumCalculation WHEN_REQUIRED", async () => {
  const c = makeClient({ requestChecksumCalculation: "WHEN_REQUIRED" });
  const url = await getSignedUrl(c, new PutObjectCommand({ Bucket, Key: "put me", ContentType: "text/plain" }), { expiresIn: 300 });
  const res = await fetch(url, { method: "PUT", body: "uploaded via presign", headers: { "Content-Type": "text/plain" } });
  assert.equal(res.status, 200, await res.text());
  const got = await client.send(new GetObjectCommand({ Bucket, Key: "put me" }));
  assert.equal((await bodyBuffer(got)).toString(), "uploaded via presign");
  assert.equal(got.ContentType, "text/plain");
});

test("expired presign is 403", async () => {
  const url = await getSignedUrl(client, new GetObjectCommand({ Bucket, Key: "get me" }), { expiresIn: 1 });
  await sleep(2000);
  const res = await fetch(url);
  assert.equal(res.status, 403);
  assert.match(await res.text(), /<Code>AccessDenied<\/Code>/);
});

test("tampered presigned signature is 403", async () => {
  const url = new URL(await getSignedUrl(client, new GetObjectCommand({ Bucket, Key: "get me" }), { expiresIn: 300 }));
  const sig = url.searchParams.get("X-Amz-Signature");
  url.searchParams.set("X-Amz-Signature", (sig[0] === "0" ? "1" : "0") + sig.slice(1));
  const res = await fetch(url);
  assert.equal(res.status, 403);
  assert.match(await res.text(), /<Code>SignatureDoesNotMatch<\/Code>/);
});

test("CORS preflight from the allowed origin", async () => {
  const res = await fetch(`${endpoint}/${Bucket}/put%20me`, {
    method: "OPTIONS",
    headers: { Origin: "http://example.test", "Access-Control-Request-Method": "PUT", "Access-Control-Request-Headers": "content-type" },
  });
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("access-control-allow-origin"), "http://example.test");
  assert.match(res.headers.get("access-control-allow-methods"), /PUT/);
  const denied = await fetch(`${endpoint}/${Bucket}/x`, {
    method: "OPTIONS", headers: { Origin: "http://evil.test", "Access-Control-Request-Method": "PUT" },
  });
  assert.equal(denied.status, 403);
});

test("actual response carries CORS headers for the allowed origin", async () => {
  const url = await getSignedUrl(client, new GetObjectCommand({ Bucket, Key: "get me" }), { expiresIn: 300 });
  const res = await fetch(url, { headers: { Origin: "http://example.test" } });
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("access-control-allow-origin"), "http://example.test");
  assert.match(res.headers.get("access-control-expose-headers"), /ETag/);
});
