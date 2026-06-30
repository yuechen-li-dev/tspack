import {
  PackageAnnotations,
  annotatePackage,
  defineDeps,
  dep,
  npm,
  peer,
  tool,
} from "tspack/manifest";

const deps = defineDeps({
  clsx: dep(npm("clsx", "^2.1.1")),
  react: peer(npm("react", "^19.0.0")),
  typescript: tool(npm("typescript", "^5.9.0")),
});

export default annotatePackage(
  <PackageAnnotations
    name="@acme/ui"
    dependencies={{ values: [deps.clsx, deps.react, deps.typescript] }}
  />,
);
