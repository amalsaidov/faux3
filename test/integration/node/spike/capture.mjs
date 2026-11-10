// Wire-capture spike: issue each upload transport once against faux3 so the
// server debug log records the payload mode headers. Faux3 answers 501 to
// everything at this stage, so errors are expected and only reported.
import { Readable } from "node:stream";
import { createRequire } from "node:module";
import { S3Client, PutObjectCommand, GetObjectCommand } from "@aws-sdk/client-s3";
import { Upload } from "@aws-sdk/lib-storage";
import { getSignedUrl } from "@aws-sdk/s3-request-presigner";

const require = createRequire(import.meta.url);
const sdkVersion = require("@aws-sdk/client-s3/package.json").version;

const client = new S3Client({
  endpoint: process.env.S3_ENDPOINT ?? "http://faux3:9000",
  region: process.env.AWS_REGION ?? "us-east-1",
  forcePathStyle: true,
  maxAttempts: 1,
});

const Bucket = "spike";
const body = "hello faux3 wire capture\n";

const cases = [
  ["put-buffer", () => client.send(new PutObjectCommand({ Bucket, Key: "buffer", Body: Buffer.from(body) }))],
  ["put-string", () => client.send(new PutObjectCommand({ Bucket, Key: "string", Body: body }))],
  ["put-stream", () => client.send(new PutObjectCommand({
    Bucket, Key: "stream", Body: Readable.from([Buffer.from(body)]), ContentLength: Buffer.byteLength(body),
  }))],
  ["lib-storage-upload", () => new Upload({ client, params: { Bucket, Key: "upload", Body: Buffer.from(body) } }).done()],
  ["presigned-put", async () => {
    const url = await getSignedUrl(client, new PutObjectCommand({ Bucket, Key: "presigned" }), { expiresIn: 300 });
    const res = await fetch(url, { method: "PUT", body });
    return { status: res.status };
  }],
  ["get-checksum-mode", () => client.send(new GetObjectCommand({ Bucket, Key: "buffer", ChecksumMode: "ENABLED" }))],
];

console.log(`node ${process.version} @aws-sdk/client-s3 ${sdkVersion}`);
for (const [name, fn] of cases) {
  try {
    const out = await fn();
    console.log(`${name}: ok ${JSON.stringify(out?.status ?? out?.$metadata?.httpStatusCode ?? "")}`);
  } catch (err) {
    console.log(`${name}: ${err.name} ${err.$metadata?.httpStatusCode ?? ""} (expected)`);
  }
}
