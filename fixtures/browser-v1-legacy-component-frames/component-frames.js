// Deliberate browser-v1 compatibility fixture. New projects must emit the V1
// default envelope instead of importing this deprecated registration API.
import { registerComponentFrames } from "@copeland/browser-v1";

registerComponentFrames([]);
