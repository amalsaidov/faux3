import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { Readable } from "node:stream";
import {
  PutObjectCommand, GetObjectCommand, HeadObjectCommand, DeleteObjectCommand,
} from "@aws-sdk/client-s3";
import { Upload } from "@aws-sdk/lib-storage";
import { client, makeClient, uniqueBucket, createBucket, emptyAndDelete, expectS3Error, bodyBuffer, closeClients } from "./lib.mjs";

const Bucket = uniqueBucket("objects");
before(() => createBucket(Bucket));
after(async () => {
  await emptyAndDelete(Bucket);
  closeClients();
});

const md5hex = (b) => createHash("md5").update(b).digest("hex");

async function roundTrip(Key, Body, expected) {
  const put = await client.send(new PutObjectCommand({ Bucket, Key, Body, ...(Body instanceof Readable ? { ContentLength: expected.length } : {}) }));
  assert.equal(put.ETag, `"${md5hex(expected)}"`);
  const got = await client.send(new GetObjectCommand({ Bucket, Key }));
  assert.deepEqual(await bodyBuffer(got), expected);
  assert.equal(got.ETag, put.ETag);
  assert.equal(got.ContentLength, expected.length);
}

test("put/get Buffer", () => roundTrip("buffer", Buffer.from("hello buffer"), Buffer.from("hello buffer")));
test("put/get string", () => roundTrip("string", "hello string", Buffer.from("hello string")));
test("put/get Readable with ContentLength", () => {
  const data = Buffer.alloc(300 * 1024, 7);
  return roundTrip("stream", Readable.from([data.subarray(0, 1000), data.subarray(1000)]), data);
});

test("lib-storage Upload under the multipart threshold", async () => {
  const data = Buffer.alloc(1024 * 1024, 3);
  await new Upload({ client, params: { Bucket, Key: "upload", Body: data } }).done();
  const got = await client.send(new GetObjectCommand({ Bucket, Key: "upload" }));
  assert.deepEqual(await bodyBuffer(got), data);
});

test("head and delete", async () => {
  await client.send(new PutObjectCommand({ Bucket, Key: "hd", Body: "abc" }));
  const head = await client.send(new HeadObjectCommand({ Bucket, Key: "hd" }));
  assert.equal(head.ContentLength, 3);
  assert.ok(head.LastModified instanceof Date);
  await client.send(new DeleteObjectCommand({ Bucket, Key: "hd" }));
  await expectS3Error(assert, client.send(new HeadObjectCommand({ Bucket, Key: "hd" })), "NotFound", 404);
  await expectS3Error(assert, client.send(new GetObjectCommand({ Bucket, Key: "hd" })), "NoSuchKey", 404);
  // Deleting a missing key succeeds.
  await client.send(new DeleteObjectCommand({ Bucket, Key: "hd" }));
});

test("metadata and content headers round-trip", async () => {
  await client.send(new PutObjectCommand({
    Bucket, Key: "meta", Body: "m",
    ContentType: "text/x-faux3", ContentDisposition: "attachment; filename=\"m.txt\"",
    ContentEncoding: "identity", ContentLanguage: "en", CacheControl: "max-age=60",
    Expires: new Date("2030-01-01T00:00:00Z"),
    Metadata: { color: "blue", "mixed-case": "Value With Spaces" },
  }));
  const head = await client.send(new HeadObjectCommand({ Bucket, Key: "meta" }));
  assert.equal(head.ContentType, "text/x-faux3");
  assert.equal(head.ContentDisposition, "attachment; filename=\"m.txt\"");
  assert.equal(head.ContentLanguage, "en");
  assert.equal(head.CacheControl, "max-age=60");
  assert.equal(head.Expires?.toISOString(), "2030-01-01T00:00:00.000Z");
  assert.deepEqual(head.Metadata, { color: "blue", "mixed-case": "Value With Spaces" });
});

test("response-* overrides", async () => {
  const got = await client.send(new GetObjectCommand({
    Bucket, Key: "meta",
    ResponseContentType: "application/json", ResponseContentDisposition: "inline",
    ResponseCacheControl: "no-cache", ResponseContentLanguage: "fr",
  }));
  assert.equal(got.ContentType, "application/json");
  assert.equal(got.ContentDisposition, "inline");
  assert.equal(got.CacheControl, "no-cache");
  assert.equal(got.ContentLanguage, "fr");
});

test("Range requests: 206 and 416", async () => {
  await client.send(new PutObjectCommand({ Bucket, Key: "range", Body: "0123456789" }));
  const got = await client.send(new GetObjectCommand({ Bucket, Key: "range", Range: "bytes=2-5" }));
  assert.equal(got.$metadata.httpStatusCode, 206);
  assert.equal(got.ContentRange, "bytes 2-5/10");
  assert.equal((await bodyBuffer(got)).toString(), "2345");
  const tail = await client.send(new GetObjectCommand({ Bucket, Key: "range", Range: "bytes=-3" }));
  assert.equal((await bodyBuffer(tail)).toString(), "789");
  await expectS3Error(assert, client.send(new GetObjectCommand({ Bucket, Key: "range", Range: "bytes=20-30" })), "InvalidRange", 416);
});

test("conditionals: 304 and 412", async () => {
  const put = await client.send(new PutObjectCommand({ Bucket, Key: "cond", Body: "c" }));
  await assert.rejects(client.send(new GetObjectCommand({ Bucket, Key: "cond", IfNoneMatch: put.ETag })),
    (err) => { assert.equal(err.$metadata.httpStatusCode, 304); return true; });
  await expectS3Error(assert, client.send(new GetObjectCommand({ Bucket, Key: "cond", IfMatch: "\"nope\"" })), "PreconditionFailed", 412);
  const ok = await client.send(new GetObjectCommand({ Bucket, Key: "cond", IfMatch: put.ETag }));
  assert.equal((await bodyBuffer(ok)).toString(), "c");
  await expectS3Error(assert, client.send(new PutObjectCommand({ Bucket, Key: "cond", Body: "d", IfNoneMatch: "*" })), "PreconditionFailed", 412);
});

test("default CRC32 checksum is stored and echoed in checksum mode", async () => {
  const put = await client.send(new PutObjectCommand({ Bucket, Key: "crc-default", Body: "checksum me" }));
  assert.ok(put.ChecksumCRC32, "server echoes the CRC32 on PUT");
  const got = await client.send(new GetObjectCommand({ Bucket, Key: "crc-default", ChecksumMode: "ENABLED" }));
  assert.equal(got.ChecksumCRC32, put.ChecksumCRC32);
  assert.equal((await bodyBuffer(got)).toString(), "checksum me");
  const head = await client.send(new HeadObjectCommand({ Bucket, Key: "crc-default", ChecksumMode: "ENABLED" }));
  assert.equal(head.ChecksumCRC32, put.ChecksumCRC32);
});

for (const [algo, field] of [["CRC32C", "ChecksumCRC32C"], ["SHA1", "ChecksumSHA1"], ["SHA256", "ChecksumSHA256"], ["CRC64NVME", "ChecksumCRC64NVME"]]) {
  test(`explicit ${algo} checksum`, async () => {
    const Key = `algo-${algo}`;
    const put = await client.send(new PutObjectCommand({ Bucket, Key, Body: `body for ${algo}`, ChecksumAlgorithm: algo }));
    assert.ok(put[field], `${field} echoed`);
    const got = await client.send(new GetObjectCommand({ Bucket, Key, ChecksumMode: "ENABLED" }));
    assert.equal(got[field], put[field]);
    assert.equal((await bodyBuffer(got)).toString(), `body for ${algo}`);
  });
}

test("Content-MD5 is validated", async () => {
  const body = Buffer.from("md5 body");
  const ContentMD5 = createHash("md5").update(body).digest("base64");
  await client.send(new PutObjectCommand({ Bucket, Key: "md5", Body: body, ContentMD5 }));
  const wrong = createHash("md5").update("other").digest("base64");
  await expectS3Error(assert, client.send(new PutObjectCommand({ Bucket, Key: "md5", Body: body, ContentMD5: wrong })), "BadDigest", 400);
});

test("tampered checksum header is BadDigest", async () => {
  const c = makeClient();
  c.middlewareStack.add((next) => async (args) => {
    args.request.headers["x-amz-checksum-sha256"] = createHash("sha256").update("not the body").digest("base64");
    return next(args);
  }, { step: "build", priority: "low", name: "tamperChecksum" });
  await expectS3Error(assert,
    c.send(new PutObjectCommand({ Bucket, Key: "tampered", Body: "real body", ChecksumSHA256: createHash("sha256").update("real body").digest("base64") })),
    "BadDigest", 400);
});
