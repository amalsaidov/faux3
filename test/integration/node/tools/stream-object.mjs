// Streams a large deterministic object to or from faux3 and prints its
// sha256. Used by persistence.sh and memory.sh; not a test file.
//
//   node tools/stream-object.mjs put <bucket> <key> <bytes> [delay-ms-per-MiB]
//   node tools/stream-object.mjs get <bucket> <key>
//
// put generates the body on the fly so neither side holds it in memory.
import { createHash } from "node:crypto";
import { Readable } from "node:stream";
import { setTimeout as sleep } from "node:timers/promises";
import { PutObjectCommand, GetObjectCommand, CreateBucketCommand } from "@aws-sdk/client-s3";
import { makeClient, closeClients } from "../lib.mjs";

const [op, Bucket, Key, sizeArg, delayArg] = process.argv.slice(2);
const client = makeClient();
const MiB = 1024 * 1024;

async function* generate(size, delayMs, hash) {
  // A cheap xorshift stream: incompressible enough and reproducible.
  let x = 0x9e3779b9;
  for (let sent = 0; sent < size;) {
    const n = Math.min(MiB, size - sent);
    const buf = Buffer.allocUnsafe(n);
    for (let i = 0; i < n; i += 4) {
      x ^= x << 13; x ^= x >>> 17; x ^= x << 5;
      buf.writeUInt32LE(x >>> 0, i);
    }
    hash.update(buf);
    sent += n;
    yield buf;
    if (delayMs > 0) await sleep(delayMs);
  }
}

try {
  if (op === "put") {
    const size = Number(sizeArg);
    const hash = createHash("sha256");
    await client.send(new CreateBucketCommand({ Bucket })).catch((err) => {
      if (err.name !== "BucketAlreadyOwnedByYou") throw err;
    });
    await client.send(new PutObjectCommand({
      Bucket, Key, ContentLength: size, Body: Readable.from(generate(size, Number(delayArg ?? 0), hash)),
    }));
    console.log(hash.digest("hex"));
  } else if (op === "get") {
    const out = await client.send(new GetObjectCommand({ Bucket, Key }));
    const hash = createHash("sha256");
    for await (const chunk of out.Body) hash.update(chunk);
    console.log(hash.digest("hex"));
  } else {
    throw new Error(`unknown op ${op}`);
  }
} finally {
  closeClients();
}
