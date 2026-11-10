// Print presigned PUT and GET URLs for one key, then use them with fetch.
import { CreateBucketCommand, GetObjectCommand, PutObjectCommand } from "@aws-sdk/client-s3";
import { getSignedUrl } from "@aws-sdk/s3-request-presigner";
import { bucket, makeClient } from "./client.mjs";

// WHEN_REQUIRED stops the presigner from hoisting a checksum of an empty body
// into the URL, which would make any real upload fail with BadDigest.
const s3 = makeClient({ requestChecksumCalculation: "WHEN_REQUIRED" });
const key = process.argv[2] ?? "presigned/example.txt";

try {
  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
} catch (err) {
  if (err.name !== "BucketAlreadyOwnedByYou") throw err;
}

const putUrl = await getSignedUrl(s3, new PutObjectCommand({ Bucket: bucket, Key: key }), { expiresIn: 900 });
const getUrl = await getSignedUrl(s3, new GetObjectCommand({ Bucket: bucket, Key: key }), { expiresIn: 900 });
console.log(`PUT ${putUrl}`);
console.log(`GET ${getUrl}`);

const put = await fetch(putUrl, { method: "PUT", body: "uploaded with a presigned URL\n" });
if (!put.ok) throw new Error(`presigned PUT failed: ${put.status} ${await put.text()}`);
const get = await fetch(getUrl);
if (!get.ok) throw new Error(`presigned GET failed: ${get.status}`);
console.log(`fetched: ${(await get.text()).trim()}`);
console.log("ok");
