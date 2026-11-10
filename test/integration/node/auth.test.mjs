import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { ListBucketsCommand, PutObjectCommand } from "@aws-sdk/client-s3";
import { client, makeClient, endpoint, uniqueBucket, createBucket, emptyAndDelete, expectS3Error, closeClients } from "./lib.mjs";

const Bucket = uniqueBucket("auth");
before(() => createBucket(Bucket));
after(async () => {
  await emptyAndDelete(Bucket);
  closeClients();
});

test("wrong secret is 403 SignatureDoesNotMatch", async () => {
  const c = makeClient({ credentials: { accessKeyId: "faux3", secretAccessKey: "wrong" } });
  await expectS3Error(assert, c.send(new ListBucketsCommand({})), "SignatureDoesNotMatch", 403);
});

test("unknown key is 403 InvalidAccessKeyId", async () => {
  const c = makeClient({ credentials: { accessKeyId: "nobody", secretAccessKey: "faux3" } });
  await expectS3Error(assert, c.send(new ListBucketsCommand({})), "InvalidAccessKeyId", 403);
});

test("wrong region is 400 AuthorizationHeaderMalformed", async () => {
  const c = makeClient({ region: "eu-west-1" });
  await expectS3Error(assert, c.send(new ListBucketsCommand({})), "AuthorizationHeaderMalformed", 400);
});

test("anonymous fetch is 403", async () => {
  const res = await fetch(`${endpoint}/`);
  assert.equal(res.status, 403);
  assert.match(await res.text(), /<Code>AccessDenied<\/Code>/);
});

test("header tampered after signing is 403", async () => {
  const c = makeClient();
  c.middlewareStack.add((next) => async (args) => {
    args.request.headers["content-type"] = "application/x-tampered";
    return next(args);
  }, { step: "finalizeRequest", priority: "low", name: "tamperAfterSign" });
  await expectS3Error(assert, c.send(new PutObjectCommand({ Bucket, Key: "t", Body: "t", ContentType: "text/plain" })), "SignatureDoesNotMatch", 403);
});

test("correct credentials still work", async () => {
  const out = await client.send(new ListBucketsCommand({}));
  assert.equal(out.$metadata.httpStatusCode, 200);
});
