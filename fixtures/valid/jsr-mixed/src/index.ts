import * as esbuildPlugin from "@jsr/deno__esbuild-plugin";
import * as flag from "@jsr/luca__flag";
import { join } from "@jsr/std__path";
import { basename } from "@jsr/std__path/basename";
import pc from "picocolors";

export const joined = join("registry", "package");
export const base = basename("registry/package.ts");
export const colored = pc.green(joined);
export const jsrModules = { esbuildPlugin, flag };
