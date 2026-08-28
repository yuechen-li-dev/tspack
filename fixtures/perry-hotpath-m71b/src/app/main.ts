import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const executable = new URL("../hotpath.exe", import.meta.url);
const payload = JSON.stringify({ values: [1, 2, 3, Date.now() & 255] });
const completed = spawnSync(fileURLToPath(executable), ["--probe", payload], { encoding: "utf8" });
if (completed.status !== 0) {
	throw new Error(`Perry sidecar failed (${completed.status}): ${completed.stderr}`);
}
const report = JSON.parse(completed.stdout);
console.log(JSON.stringify({ bridge: "nativeExecutable", payloadBytes: report.bytes, checksum: report.checksum }));
