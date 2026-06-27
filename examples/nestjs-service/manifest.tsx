import {
  Env,
  Package,
  RunTargets,
  Service,
  Tools,
  Workspace,
  define,
  defineDeps,
  npm,
  tool,
} from "tspack/manifest";

const deps = defineDeps({
  nestjsCommon: tool(npm("@nestjs/common", "^10.4.20"), {
    key: "@nestjs/common",
  }),
  nestjsCore: tool(npm("@nestjs/core", "^10.4.20"), { key: "@nestjs/core" }),
  nestjsPlatformExpress: tool(npm("@nestjs/platform-express", "^10.4.20"), {
    key: "@nestjs/platform-express",
  }),
  reflectMetadata: tool(npm("reflect-metadata", "^0.2.2"), {
    key: "reflect-metadata",
  }),
  rxjs: tool(npm("rxjs", "^7.8.2")),
  typesNode: tool(npm("@types/node", "^22.15.30"), { key: "@types/node" }),
  biome: tool(npm("@biomejs/biome", "^1.9.4"), { key: "@biomejs/biome" }),
  tsx: tool(npm("tsx", "^4.20.3")),
  typescript: tool(npm("typescript", "^5.8.3")),
  vitest: tool(npm("vitest", "^3.2.4")),
});

export default define(
  <Workspace name="nestjs-service-example" runtime="nodejs">
    <Package
      name="@tspack-examples/nestjs-service"
      version="0.1.0"
      license="MIT"
      kind="service"
      dependencies={{
        values: [
          deps.nestjsCommon,
          deps.nestjsCore,
          deps.nestjsPlatformExpress,
          deps.reflectMetadata,
          deps.rxjs,
          deps.typesNode,
          deps.biome,
          deps.tsx,
          deps.typescript,
          deps.vitest,
        ],
      }}
    >
      <Tools
        values={[
          deps.nestjsCommon,
          deps.nestjsCore,
          deps.nestjsPlatformExpress,
          deps.reflectMetadata,
          deps.rxjs,
          deps.typesNode,
          deps.biome,
          deps.tsx,
          deps.typescript,
          deps.vitest,
        ]}
      />
      <RunTargets
        rows={[
          {
            name: "dev",
            runtime: "system",
            cwd: "workspace",
            command: ["node_modules/.bin/tsx", "watch", "src/main.ts"],
            url: "http://127.0.0.1:${PORT}",
            ready: { kind: "http", path: "/health" },
            env: [
              Env("PORT", {
                default: "3000",
                description: "HTTP port for the NestJS service",
              }),
              Env("NODE_ENV", {
                default: "development",
                description: "Node environment for local development",
              }),
              Env("DATABASE_URL", {
                secret: true,
                description:
                  "Optional future database connection string; not used by this minimal service",
              }),
            ],
            requires: [
              Service("postgres", {
                tcp: "127.0.0.1:5432",
                optional: true,
                description:
                  "Optional local Postgres used by future DB examples",
              }),
            ],
          },
          {
            name: "build",
            runtime: "system",
            url: "http://127.0.0.1:3000",
            cwd: "workspace",
            command: ["node_modules/.bin/tsc", "-p", "tsconfig.build.json"],
          },
          {
            name: "start",
            runtime: "system",
            cwd: "workspace",
            command: ["node", "dist/main.js"],
            url: "http://127.0.0.1:${PORT}",
            ready: { kind: "http", path: "/health" },
            env: [
              Env("PORT", {
                default: "3000",
                description: "HTTP port for the built NestJS service",
              }),
              Env("NODE_ENV", {
                default: "production",
                description: "Node environment for the built service",
              }),
            ],
          },
          {
            name: "typecheck",
            runtime: "system",
            url: "http://127.0.0.1:3000",
            cwd: "workspace",
            command: [
              "node_modules/.bin/tsc",
              "-p",
              "tsconfig.json",
              "--noEmit",
            ],
          },
          {
            name: "lint",
            runtime: "system",
            url: "http://127.0.0.1:3000",
            cwd: "workspace",
            command: ["node_modules/.bin/biome", "check", "."],
          },
          {
            name: "test",
            runtime: "system",
            url: "http://127.0.0.1:3000",
            cwd: "workspace",
            command: ["node_modules/.bin/vitest", "run"],
          },
        ]}
      />
    </Package>
  </Workspace>,
);
