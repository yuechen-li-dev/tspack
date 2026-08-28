import { defineTypeScriptWorkspace } from "copeland/workspace";

export default defineTypeScriptWorkspace({
  ownership: "partial",
  tscl: {
    project: "./App.csproj",
    include: ["src/**"],
  },
});
