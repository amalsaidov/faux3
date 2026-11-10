import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { PutObjectCommand, GetObjectCommand, ListObjectsV2Command } from "@aws-sdk/client-s3";
import { client, uniqueBucket, createBucket, emptyAndDelete, expectS3Error, bodyBuffer, closeClients } from "./lib.mjs";

const Bucket = uniqueBucket("keys");
before(() => createBucket(Bucket));
after(async () => {
  await emptyAndDelete(Bucket);
  closeClients();
});

const keys = [
  "a b",
  "uni/caf\u00e9",
  "emoji/\u{1F600}",
  "plus+sign",
  "percent%2Fliteral",
  "dots/../up",
  "..",
  "a//b",
  "/leading",
  "trailing/",
  "q?mark&amp=eq",
  "k".repeat(1024),
];

for (const Key of keys) {
  test(`key round-trips: ${JSON.stringify(Key.length > 40 ? Key.slice(0, 20) + "..." : Key)}`, async () => {
    await client.send(new PutObjectCommand({ Bucket, Key, Body: Key }));
    const got = await client.send(new GetObjectCommand({ Bucket, Key }));
    assert.equal((await bodyBuffer(got)).toString(), Key);
  });
}

test("listing returns every unusual key verbatim", async () => {
  const listed = [];
  let ContinuationToken;
  do {
    const page = await client.send(new ListObjectsV2Command({ Bucket, ContinuationToken }));
    listed.push(...(page.Contents ?? []).map((o) => o.Key));
    ContinuationToken = page.NextContinuationToken;
  } while (ContinuationToken);
  for (const k of keys) assert.ok(listed.includes(k), `listed ${JSON.stringify(k.slice(0, 40))}`);
});

test("1025-byte key is rejected", async () => {
  await expectS3Error(assert, client.send(new PutObjectCommand({ Bucket, Key: "k".repeat(1025), Body: "x" })), "KeyTooLongError", 400);
});

test("binary body with all 256 byte values", async () => {
  const body = Buffer.alloc(256 * 4);
  for (let i = 0; i < body.length; i++) body[i] = i % 256;
  await client.send(new PutObjectCommand({ Bucket, Key: "binary", Body: body }));
  const got = await client.send(new GetObjectCommand({ Bucket, Key: "binary" }));
  assert.deepEqual(await bodyBuffer(got), body);
});

test("zero-byte body", async () => {
  await client.send(new PutObjectCommand({ Bucket, Key: "empty", Body: Buffer.alloc(0) }));
  const got = await client.send(new GetObjectCommand({ Bucket, Key: "empty" }));
  assert.equal(got.ContentLength, 0);
  assert.equal((await bodyBuffer(got)).length, 0);
  assert.equal(got.ETag, "\"d41d8cd98f00b204e9800998ecf8427e\"");
});
