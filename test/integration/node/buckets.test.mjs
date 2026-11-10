import { test, after } from "node:test";
import assert from "node:assert/strict";
import {
  CreateBucketCommand, DeleteBucketCommand, HeadBucketCommand, ListBucketsCommand, PutObjectCommand,
  DeleteObjectCommand, GetBucketLocationCommand,
} from "@aws-sdk/client-s3";
import { client, uniqueBucket, expectS3Error, closeClients } from "./lib.mjs";

const Bucket = uniqueBucket("buckets");
after(async () => {
  await client.send(new DeleteObjectCommand({ Bucket, Key: "x" })).catch(() => {});
  await client.send(new DeleteBucketCommand({ Bucket })).catch(() => {});
  closeClients();
});

test("create, list, head, location, delete", async () => {
  await client.send(new CreateBucketCommand({ Bucket }));
  const list = await client.send(new ListBucketsCommand({}));
  assert.ok(list.Buckets.some((b) => b.Name === Bucket));
  assert.ok(list.Owner?.ID);
  const head = await client.send(new HeadBucketCommand({ Bucket }));
  assert.equal(head.BucketRegion, "us-east-1");
  const loc = await client.send(new GetBucketLocationCommand({ Bucket }));
  assert.ok(loc.LocationConstraint === undefined || loc.LocationConstraint === "");
});

test("duplicate create is 409", async () => {
  await expectS3Error(assert, client.send(new CreateBucketCommand({ Bucket })), "BucketAlreadyOwnedByYou", 409);
});

test("create with matching LocationConstraint", async () => {
  const b = uniqueBucket("loc");
  // us-east-1 rejects an explicit constraint in real S3; faux3 accepts its own region.
  await client.send(new CreateBucketCommand({ Bucket: b, CreateBucketConfiguration: { LocationConstraint: "us-east-1" } }));
  await client.send(new DeleteBucketCommand({ Bucket: b }));
  await expectS3Error(assert,
    client.send(new CreateBucketCommand({ Bucket: b, CreateBucketConfiguration: { LocationConstraint: "eu-west-1" } })),
    "InvalidLocationConstraint", 400);
});

test("delete non-empty is 409", async () => {
  await client.send(new PutObjectCommand({ Bucket, Key: "x", Body: "x" }));
  await expectS3Error(assert, client.send(new DeleteBucketCommand({ Bucket })), "BucketNotEmpty", 409);
  await client.send(new DeleteObjectCommand({ Bucket, Key: "x" }));
});

test("invalid bucket names are rejected", async () => {
  for (const name of ["ab", "UPPER-case", "bad_underscore", "-dash", "a".repeat(64), "192.168.1.1", "dots..dots"]) {
    await assert.rejects(client.send(new CreateBucketCommand({ Bucket: name })),
      (err) => { assert.equal(err.$metadata?.httpStatusCode, 400, name); return true; }, name);
  }
});

test("missing bucket: head 404, delete 404", async () => {
  const missing = uniqueBucket("missing");
  await expectS3Error(assert, client.send(new HeadBucketCommand({ Bucket: missing })), "NotFound", 404);
  await expectS3Error(assert, client.send(new DeleteBucketCommand({ Bucket: missing })), "NoSuchBucket", 404);
});

test("delete bucket", async () => {
  await client.send(new DeleteBucketCommand({ Bucket }));
  const list = await client.send(new ListBucketsCommand({}));
  assert.ok(!(list.Buckets ?? []).some((b) => b.Name === Bucket));
});
