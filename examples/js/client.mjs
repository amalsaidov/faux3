// Shared S3 client configured for a local faux3 endpoint.
import { S3Client } from "@aws-sdk/client-s3";

export const bucket = process.env.S3_BUCKET ?? "example";

export function makeClient(overrides = {}) {
  return new S3Client({
    endpoint: process.env.S3_ENDPOINT ?? "http://localhost:9000",
    region: process.env.AWS_REGION ?? "us-east-1",
    forcePathStyle: true,
    credentials: {
      accessKeyId: process.env.AWS_ACCESS_KEY_ID ?? "faux3",
      secretAccessKey: process.env.AWS_SECRET_ACCESS_KEY ?? "faux3",
    },
    ...overrides,
  });
}
