// Shared helpers for the node integration tests. Not a test file itself.
import { randomBytes } from "node:crypto";
import {
  S3Client, CreateBucketCommand, DeleteBucketCommand, DeleteObjectCommand, ListObjectsV2Command,
} from "@aws-sdk/client-s3";

export const endpoint = process.env.S3_ENDPOINT ?? "http://faux3:9000";
export const region = process.env.AWS_REGION ?? "us-east-1";

const clients = [];

export function makeClient(overrides = {}) {
  const c = new S3Client({ endpoint, region, forcePathStyle: true, maxAttempts: 1, ...overrides });
  clients.push(c);
  return c;
}

// closeClients destroys every client's keep-alive sockets so the test
// process exits promptly instead of waiting for the server idle timeout.
export function closeClients() {
  for (const c of clients) c.destroy();
}

export const client = makeClient();

export function uniqueBucket(prefix) {
  return `${prefix}-${randomBytes(4).toString("hex")}`;
}

export async function createBucket(Bucket) {
  await client.send(new CreateBucketCommand({ Bucket }));
  return Bucket;
}

// emptyAndDelete removes every object, then the bucket.
export async function emptyAndDelete(Bucket) {
  let ContinuationToken;
  do {
    const page = await client.send(new ListObjectsV2Command({ Bucket, ContinuationToken }));
    for (const o of page.Contents ?? []) {
      await client.send(new DeleteObjectCommand({ Bucket, Key: o.Key }));
    }
    ContinuationToken = page.NextContinuationToken;
  } while (ContinuationToken);
  await client.send(new DeleteBucketCommand({ Bucket }));
}

// expectS3Error awaits p and asserts it rejects with the given code and status.
export async function expectS3Error(assert, p, code, status) {
  await assert.rejects(p, (err) => {
    if (code !== undefined) assert.equal(err.name, code, `error name ${err.name}: ${err.message}`);
    if (status !== undefined) assert.equal(err.$metadata?.httpStatusCode, status);
    return true;
  });
}

export async function bodyBuffer(out) {
  return Buffer.from(await out.Body.transformToByteArray());
}
