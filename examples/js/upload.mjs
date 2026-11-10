// Create a bucket if missing, put an object, read it back, and list the bucket.
import {
  CreateBucketCommand,
  GetObjectCommand,
  HeadBucketCommand,
  ListObjectsV2Command,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import { bucket, makeClient } from "./client.mjs";

const s3 = makeClient();
const key = "hello/world.txt";
const body = `hello from faux3 at ${new Date().toISOString()}\n`;

async function ensureBucket() {
  try {
    await s3.send(new HeadBucketCommand({ Bucket: bucket }));
  } catch (err) {
    if (err.$metadata?.httpStatusCode !== 404) throw err;
    await s3.send(new CreateBucketCommand({ Bucket: bucket }));
    console.log(`created bucket ${bucket}`);
  }
}

await ensureBucket();
await s3.send(new PutObjectCommand({ Bucket: bucket, Key: key, Body: body, ContentType: "text/plain" }));
console.log(`put s3://${bucket}/${key}`);

const got = await s3.send(new GetObjectCommand({ Bucket: bucket, Key: key }));
const text = await got.Body.transformToString();
if (text !== body) throw new Error(`round trip mismatch: ${JSON.stringify(text)}`);
console.log(`get s3://${bucket}/${key}: ${text.trim()}`);

const listed = await s3.send(new ListObjectsV2Command({ Bucket: bucket }));
for (const obj of listed.Contents ?? []) console.log(`list ${obj.Key} ${obj.Size} bytes`);
console.log("ok");
