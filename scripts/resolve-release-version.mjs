import { appendFileSync } from "node:fs";

const version = process.env.REQUESTED_VERSION || process.env.RELEASED_VERSION || "";
if (version && !/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
  throw new Error(`invalid semantic version: ${version}`);
}
if (!process.env.GITHUB_OUTPUT) throw new Error("GITHUB_OUTPUT is unavailable");
appendFileSync(process.env.GITHUB_OUTPUT, `version=${version}\n`);
