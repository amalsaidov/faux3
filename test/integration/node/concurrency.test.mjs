import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { PutObjectCommand, GetObjectCommand, NoSuchKey } from "@aws-sdk/client-s3";
import { client, uniqueBucket, createBucket, emptyAndDelete, bodyBuffer, closeClients } from "./lib.mjs";

const Bucket = uniqueBucket("concurrency");
before(() => createBucket(Bucket));
after(async () => {
  await emptyAndDelete(Bucket);
  closeClients();
});

const SIZE = 1024 * 1024;
const WRITERS = 20;
const READERS = 20;

test("parallel writers and readers on one key never see a torn object", async () => {
  const bodies = Array.from({ length: WRITERS }, (_, i) => Buffer.alloc(SIZE, i + 1));
  const etags = new Map(bodies.map((b, i) => [`"${createHash("md5").update(b).digest("hex")}"`, i + 1]));
  await client.send(new PutObjectCommand({ Bucket, Key: "hot", Body: bodies[0] }));

  const writers = bodies.map((Body) => client.send(new PutObjectCommand({ Bucket, Key: "hot", Body })));
  const readers = Array.from({ length: READERS }, async () => {
    for (let n = 0; n < 3; n++) {
      let got;
      try {
        got = await client.send(new GetObjectCommand({ Bucket, Key: "hot" }));
      } catch (err) {
        if (err instanceof NoSuchKey) assert.fail("object vanished during overwrite");
        throw err;
      }
      const buf = await bodyBuffer(got);
      assert.equal(buf.length, SIZE);
      const id = buf[0];
      assert.ok(buf.every((b) => b === id), "body is uniform");
      assert.equal(etags.get(got.ETag), id, "ETag matches the body");
    }
  });
  await Promise.all([...writers, ...readers]);
});
