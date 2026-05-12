import { define } from "tspack/manifest";
const version = process.env.VERSION;
export default define(<Workspace name={version} />);
