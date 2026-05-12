import { define } from "tspack/manifest";
function helper() { return "x"; }
export default define(<Workspace name={helper()} />);
