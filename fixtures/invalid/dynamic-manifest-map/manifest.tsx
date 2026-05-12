import { define } from "tspack/manifest";
const rows = ["core"].map((name) => ({ name }));
export default define(<Workspace name="x"><Package name="p" version="1.0.0" license="MIT" kind="library"><Targets rows={rows} /></Package></Workspace>);
