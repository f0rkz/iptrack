import { appendFileSync } from "node:fs";

const version = process.argv[2] ?? "";
const output = process.env.GITHUB_OUTPUT;
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
  throw new Error(`semantic-release returned an invalid version: ${version}`);
}
if (!output) throw new Error("GITHUB_OUTPUT is unavailable");
appendFileSync(output, `version=${version}\n`);
