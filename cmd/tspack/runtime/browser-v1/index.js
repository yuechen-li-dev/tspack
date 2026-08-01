export function setText(id, text) {
  const element = document.getElementById(id);
  if (element === null) throw new Error("Copeland browser host could not find text element '" + id + "'.");
  element.textContent = text;
}

export function onClick(id, callback) {
  const button = document.getElementById(id);
  if (!(button instanceof HTMLButtonElement)) throw new Error("Copeland browser host expected button '" + id + "'.");
  button.addEventListener("click", callback);
}

export function dispatch(initialState, reduce, render) {
  let currentState = initialState;
  render(currentState);
  return event => {
    const nextState = reduce(currentState, event);
    if (nextState !== currentState) {
      currentState = nextState;
      render(currentState);
    }
  };
}

export function getMountElement(id) {
  const element = document.getElementById(id);
  if (element === null) throw new Error("Copeland React host could not find mount element '" + id + "'.");
  return element;
}

export function dispatchReact(initialState, reduce, render) {
  let currentState = initialState;
  const send = event => {
    currentState = reduce(currentState, event);
    render(currentState, send);
  };
  render(currentState, send);
  return send;
}

const componentFrames = new Map();
const destroyedComponentFrames = new Map();
const componentFrameTrace = [];

// Compiler-generated modules register these definitions before the initial
// attachment artifact is loaded. State is held here, indexed solely by the
// canonical component-instance identity; adapters never receive it.
export function registerComponentFrames(frames) {
  noteLegacyRegistrationDuringModuleLoad();
  if (!Array.isArray(frames)) throw new Error("COPE-COMPONENT-STATE-BROWSER-1002 component frames must be an array");
  for (const definition of frames) {
    validateComponentFrameDefinition(definition);
    if (componentFrames.has(definition.componentInstanceId)) {
      throw new Error("COPE-COMPONENT-STATE-BROWSER-1003 duplicate component frame " + definition.componentInstanceId);
    }
    const frame = {
      ...definition,
      currentState: definition.initialState,
      lifecycle: "active"
    };
    destroyedComponentFrames.delete(frame.componentInstanceId);
    componentFrames.set(frame.componentInstanceId, frame);
    traceComponentFrame("FrameRegistered", frame);
  }
}

// Deprecated browser-v1 compatibility API. New Copeland artifacts call
// registerComponentFrameEnvelope instead. The loader records a trace rather
// than logging noisy production warnings for a valid legacy artifact.
export function recordLegacyComponentFrameContract(artifact) {
  const path = artifact !== null && typeof artifact === "object" && typeof artifact.path === "string"
    ? artifact.path
    : "component-frames.js";
  componentFrameTrace.push({
    kind: "LegacyFrameContractLoaded",
    componentInstanceId: null,
    stateIdentity: null,
    attachmentId: null,
    eventName: null,
    artifactPath: path,
    message: "Legacy component-frame registration contract loaded. Migrate this project to schemaVersion 1 before browser-v2.",
  });
}

function noteLegacyRegistrationDuringModuleLoad() {
  const artifact = globalThis.__copelandFrameArtifactLoading;
  if (artifact !== null && typeof artifact === "object") {
    artifact.legacyRegistrations = (artifact.legacyRegistrations ?? 0) + 1;
  }
}

// V1 is the generated-browser boundary. The envelope is inert compiler data;
// this runtime creates the fixed transition and projection executors.
export function registerComponentFrameEnvelope(envelope) {
  validateComponentFrameEnvelope(envelope);
  const definitions = new Map(envelope.frameDefinitions.map(definition => [definition.frameDefinitionId, definition]));
  const frames = envelope.frameInstances
    .slice()
    .sort((left, right) => left.componentInstanceId.localeCompare(right.componentInstanceId))
    .map(instance => createEnvelopeRuntimeFrame(instance, definitions.get(instance.frameDefinitionId)));
  registerComponentFrames(frames);
}

function validateComponentFrameEnvelope(envelope) {
  if (envelope === null || typeof envelope !== "object") {
    throw new Error("COPE-COMPONENT-STATE-V1-1001 component frame envelope must be an object");
  }
  if (envelope.schemaVersion !== 1) {
    throw new Error("COPE-COMPONENT-STATE-V1-1002 unsupported component frame schema version " + envelope.schemaVersion);
  }
  if (typeof envelope.projectId !== "string" || !Array.isArray(envelope.frameDefinitions) || !Array.isArray(envelope.frameInstances)) {
    throw new Error("COPE-COMPONENT-STATE-V1-1003 component frame envelope is missing projectId, frameDefinitions, or frameInstances");
  }
  const definitionIds = new Set();
  for (const definition of envelope.frameDefinitions) {
    validateFrameDefinition(definition);
    if (definitionIds.has(definition.frameDefinitionId)) {
      throw new Error("COPE-COMPONENT-STATE-V1-1004 duplicate frame definition " + definition.frameDefinitionId);
    }
    definitionIds.add(definition.frameDefinitionId);
  }
  const instanceIds = new Set();
  for (const instance of envelope.frameInstances) {
    if (instance === null || typeof instance !== "object"
      || typeof instance.componentInstanceId !== "string"
      || typeof instance.frameDefinitionId !== "string"
      || !definitionIds.has(instance.frameDefinitionId)
      || (instance.parentComponentInstanceId !== null && typeof instance.parentComponentInstanceId !== "string")) {
      throw new Error("COPE-COMPONENT-STATE-V1-1005 invalid frame instance");
    }
    if (instanceIds.has(instance.componentInstanceId)) {
      throw new Error("COPE-COMPONENT-STATE-V1-1006 duplicate frame instance " + instance.componentInstanceId);
    }
    instanceIds.add(instance.componentInstanceId);
  }
}

function validateFrameDefinition(definition) {
  if (definition === null || typeof definition !== "object"
    || typeof definition.frameDefinitionId !== "string"
    || typeof definition.componentDefinitionId !== "string"
    || typeof definition.stateIdentity !== "string"
    || !Array.isArray(definition.attachmentIds)
    || !Array.isArray(definition.events)
    || !Array.isArray(definition.presentationBranches)) {
    throw new Error("COPE-COMPONENT-STATE-V1-1007 invalid frame definition");
  }
  const eventNames = new Set();
  for (const event of definition.events) {
    if (event === null || typeof event !== "object" || typeof event.eventId !== "string" || typeof event.name !== "string"
      || event.payloadContract !== "void" || event.transition === null || typeof event.transition !== "object") {
      throw new Error("COPE-COMPONENT-STATE-V1-1008 invalid event definition frame=" + definition.frameDefinitionId);
    }
    if (eventNames.has(event.name)) {
      throw new Error("COPE-COMPONENT-STATE-V1-1009 duplicate event=" + event.name + " frame=" + definition.frameDefinitionId);
    }
    eventNames.add(event.name);
    validateTransition(event.transition, definition.frameDefinitionId, event.name);
  }
  const branchIds = new Set();
  for (const branch of definition.presentationBranches) {
    if (branch === null || typeof branch !== "object" || typeof branch.branchId !== "string" || typeof branch.statePattern !== "string" || !Array.isArray(branch.childFrames)) {
      throw new Error("COPE-COMPONENT-STATE-V1-1010 invalid presentation branch frame=" + definition.frameDefinitionId);
    }
    if (branchIds.has(branch.branchId)) {
      throw new Error("COPE-COMPONENT-STATE-V1-1011 duplicate branch=" + branch.branchId + " frame=" + definition.frameDefinitionId);
    }
    branchIds.add(branch.branchId);
    for (const child of branch.childFrames) validateEnvelopeChildFrame(child, definition.frameDefinitionId, branch.branchId);
  }
}

function validateTransition(transition, frameDefinitionId, eventName) {
  if (transition.kind === "constant" && typeof transition.nextState === "string") return;
  if (transition.kind === "match" && Array.isArray(transition.arms)
    && transition.arms.every(arm => arm !== null && typeof arm === "object" && typeof arm.statePattern === "string" && typeof arm.nextState === "string")) return;
  throw new Error("COPE-COMPONENT-STATE-V1-1012 invalid transition frame=" + frameDefinitionId + " event=" + eventName);
}

function validateEnvelopeChildFrame(child, frameDefinitionId, branchId) {
  if (child === null || typeof child !== "object" || typeof child.componentInstanceId !== "string"
    || typeof child.componentDefinitionId !== "string" || typeof child.parentComponentInstanceId !== "string"
    || typeof child.stateIdentity !== "string" || !Array.isArray(child.attachmentIds)
    || !Array.isArray(child.events) || child.attachment === null || typeof child.attachment !== "object") {
    throw new Error("COPE-COMPONENT-STATE-V1-1013 invalid child frame=" + frameDefinitionId + " branch=" + branchId);
  }
}

function createEnvelopeRuntimeFrame(instance, definition) {
  const eventContracts = {};
  for (const event of definition.events) {
    eventContracts[event.name] = {
      payload: event.payloadContract,
      transition: (_payload, currentState) => executeEnvelopeTransition(definition, event, currentState),
    };
  }
  return {
    componentInstanceId: instance.componentInstanceId,
    componentDefinitionId: definition.componentDefinitionId,
    parentComponentInstanceId: instance.parentComponentInstanceId,
    stateIdentity: definition.stateIdentity,
    initialState: instance.initialState,
    attachmentIds: definition.attachmentIds,
    eventContracts,
    rendererEventName: definition.rendererEventName,
    source: definition.source ?? instance.source ?? null,
    project: (state, plans) => projectEnvelopeFrame(definition, instance, state, plans),
  };
}

function executeEnvelopeTransition(definition, event, currentState) {
  const transition = event.transition;
  if (transition.kind === "constant") return transition.nextState;
  const arm = transition.arms.find(candidate => candidate.statePattern === currentState);
  if (arm === undefined) {
    throw new Error("COPE-COMPONENT-STATE-V1-1014 transition has no selected arm frame=" + definition.frameDefinitionId + " event=" + event.name + " state=" + currentState);
  }
  return arm.nextState;
}

function projectEnvelopeFrame(definition, instance, state, plans) {
  const retainedPlans = plans.map(plan => ({ ...plan, payload: { ...plan.payload, label: state } }));
  const branch = definition.presentationBranches.find(candidate => candidate.statePattern === state);
  if (branch === undefined) return { plans: retainedPlans, frames: [] };
  return {
    plans: retainedPlans,
    frames: branch.childFrames.map(child => createEnvelopeChildFrame(child, instance, retainedPlans)),
  };
}

function createEnvelopeChildFrame(child, parent, retainedPlans) {
  const parentPlan = retainedPlans[0];
  if (parentPlan === undefined) {
    throw new Error("COPE-COMPONENT-STATE-V1-1015 child frame has no parent attachment component=" + parent.componentInstanceId + " child=" + child.componentInstanceId);
  }
  const events = {};
  for (const event of child.events) {
    events[event.name] = {
      payload: event.payloadContract,
      transition: (_payload, currentState) => executeEnvelopeTransition(child, event, currentState),
    };
  }
  const plan = {
    ...parentPlan,
    attachmentId: child.attachment.attachmentId,
    componentDefinitionId: child.attachment.componentDefinitionId,
    componentInstanceId: child.componentInstanceId,
    parentComponentInstanceId: parent.componentInstanceId,
    payload: child.attachment.payload,
  };
  return {
    componentInstanceId: child.componentInstanceId,
    componentDefinitionId: child.componentDefinitionId,
    parentComponentInstanceId: parent.componentInstanceId,
    stateIdentity: child.stateIdentity,
    initialState: child.initialState,
    attachmentIds: child.attachmentIds,
    eventContracts: events,
    rendererEventName: child.rendererEventName,
    source: child.source ?? null,
    project: (state, childPlans) => ({
      plans: childPlans.map(childPlan => ({ ...childPlan, payload: { ...childPlan.payload, label: state } })),
      frames: [],
    }),
    plans: [plan],
  };
}

export function dispatchComponentEvent(componentInstanceId, eventName, payload) {
  const frame = componentFrames.get(componentInstanceId);
  if (frame === undefined) {
    const destroyed = destroyedComponentFrames.get(componentInstanceId);
    if (destroyed !== undefined) {
      throw new Error("COPE-COMPONENT-STATE-0103 event delivered to destroyed component instance component=" + componentInstanceId + " state=" + destroyed.stateIdentity);
    }
    throw new Error("COPE-COMPONENT-STATE-BROWSER-1004 event target frame is unavailable component=" + componentInstanceId);
  }
  if (frame.lifecycle !== "active") {
    throw new Error("COPE-COMPONENT-STATE-0103 event delivered to destroyed component instance component=" + componentInstanceId + " state=" + frame.stateIdentity);
  }
  const contract = frame.eventContracts[eventName];
  if (contract === undefined) {
    throw new Error("COPE-COMPONENT-STATE-BROWSER-1005 undeclared event " + eventName + " for component=" + componentInstanceId);
  }
  if (contract.payload === "void" && payload !== undefined && payload !== null) {
    throw new Error("COPE-COMPONENT-STATE-BROWSER-1006 event " + eventName + " does not accept a payload component=" + componentInstanceId);
  }

  traceComponentFrame("EventDispatched", frame, undefined, eventName);
  let nextState;
  try {
    nextState = contract.transition(payload, frame.currentState);
  } catch (error) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1007", frame, eventName, "transition failed", error);
  }
  if (nextState === undefined) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1008", frame, eventName, "transition returned undefined");
  }

  frame.currentState = nextState;
  traceComponentFrame("StateReplaced", frame, undefined, eventName);
  const currentPlans = frame.attachmentIds.map(attachmentId => registeredAttachmentPlans.get(attachmentId)).filter(plan => plan !== undefined);
  if (currentPlans.length !== frame.attachmentIds.length) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1009", frame, eventName, "initial attachment plans are unavailable");
  }
  let projection;
  try {
    projection = frame.project(nextState, currentPlans);
  } catch (error) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1010", frame, eventName, "presentation recomputation failed", error);
  }

  traceComponentFrame("PresentationRecomputed", frame, undefined, eventName);
  replaceComponentFrameProjection(frame, projection, eventName);
  return nextState;
}

export function destroyComponentFrame(componentInstanceId) {
  const frame = componentFrames.get(componentInstanceId);
  if (frame === undefined || frame.lifecycle === "destroyed") return;
  const children = [...componentFrames.values()]
    .filter(candidate => candidate.parentComponentInstanceId === componentInstanceId)
    .sort((left, right) => right.componentInstanceId.localeCompare(left.componentInstanceId));
  for (const child of children) destroyComponentFrame(child.componentInstanceId);

  const removedIds = new Set(frame.attachmentIds);
  const remaining = [...registeredAttachmentPlans.values()].filter(plan => !removedIds.has(plan.attachmentId));
  registerAttachmentPlans({ schemaVersion: 1, projectId: "component-frame-destroy", plans: remaining });
  frame.lifecycle = "destroyed";
  componentFrames.delete(componentInstanceId);
  destroyedComponentFrames.set(componentInstanceId, { stateIdentity: frame.stateIdentity });
  traceComponentFrame("FrameDestroyed", frame);
}

export function shutdownComponentFrames() {
  for (const frame of [...componentFrames.values()].sort((left, right) => right.componentInstanceId.localeCompare(left.componentInstanceId))) {
    destroyComponentFrame(frame.componentInstanceId);
  }
}

export function inspectComponentFrame(componentInstanceId) {
  const frame = componentFrames.get(componentInstanceId);
  if (frame === undefined) return null;
  return {
    componentInstanceId: frame.componentInstanceId,
    componentDefinitionId: frame.componentDefinitionId,
    stateIdentity: frame.stateIdentity,
    lifecycle: frame.lifecycle,
    attachmentIds: [...frame.attachmentIds]
  };
}

export function inspectComponentFrameTrace() {
  return componentFrameTrace.map(entry => ({ ...entry }));
}

function validateComponentFrameDefinition(definition) {
  if (definition === null || typeof definition !== "object" || typeof definition.componentInstanceId !== "string" || typeof definition.componentDefinitionId !== "string" || typeof definition.stateIdentity !== "string" || !Array.isArray(definition.attachmentIds) || definition.eventContracts === null || typeof definition.eventContracts !== "object" || typeof definition.project !== "function") {
    throw new Error("COPE-COMPONENT-STATE-BROWSER-1002 component frame has a missing required field");
  }
  for (const attachmentId of definition.attachmentIds) {
    if (typeof attachmentId !== "string") throw new Error("COPE-COMPONENT-STATE-BROWSER-1002 component frame attachment identity must be a string");
  }
  for (const [eventName, contract] of Object.entries(definition.eventContracts)) {
    if (typeof eventName !== "string" || contract === null || typeof contract !== "object" || typeof contract.payload !== "string" || typeof contract.transition !== "function") {
      throw new Error("COPE-COMPONENT-STATE-BROWSER-1002 component event contract is invalid");
    }
  }
}

function replaceComponentFrameProjection(frame, projection, eventName) {
  const normalized = normalizeComponentProjection(frame, projection, eventName);
  const priorChildren = [...componentFrames.values()]
    .filter(candidate => candidate.parentComponentInstanceId === frame.componentInstanceId)
    .sort((left, right) => right.componentInstanceId.localeCompare(left.componentInstanceId));
  const nextChildren = new Map(normalized.frames.map(child => [child.componentInstanceId, child]));

  // Removed descendants are released before their parent plans are replaced.
  // destroyComponentFrame already performs deepest-first attachment cleanup.
  for (const child of priorChildren) {
    if (!nextChildren.has(child.componentInstanceId)) {
      destroyComponentFrame(child.componentInstanceId);
      traceComponentFrame("ChildFrameDestroyed", frame, undefined, eventName);
    }
  }

  const retained = [];
  for (const childDefinition of normalized.frames) {
    const current = componentFrames.get(childDefinition.componentInstanceId);
    if (current === undefined) {
      registerProjectedChildFrame(frame, childDefinition, eventName);
      continue;
    }
    if (current.parentComponentInstanceId !== frame.componentInstanceId) {
      throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1014", frame, eventName, "projected child identity is owned by another parent");
    }
    retained.push({ frame: current, plans: childDefinition.plans });
  }

  replaceComponentFramePlans(frame, normalized.plans, eventName);
  for (const child of retained) {
    replaceComponentFramePlans(child.frame, child.plans, eventName);
    traceComponentFrame("ChildFrameRetained", child.frame, undefined, eventName);
  }
}

function normalizeComponentProjection(frame, projection, eventName) {
  if (Array.isArray(projection)) {
    return { plans: projection, frames: [] };
  }
  if (projection === null || typeof projection !== "object" || !Array.isArray(projection.plans) || !Array.isArray(projection.frames)) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1011", frame, eventName, "presentation projection must return plans and child frames");
  }
  const childIds = new Set();
  for (const child of projection.frames) {
    validateProjectedChildFrame(frame, child, eventName);
    if (childIds.has(child.componentInstanceId)) {
      throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1015", frame, eventName, "projection has duplicate child frame " + child.componentInstanceId);
    }
    childIds.add(child.componentInstanceId);
  }
  return projection;
}

function validateProjectedChildFrame(parent, child, eventName) {
  if (child === null || typeof child !== "object" || child.parentComponentInstanceId !== parent.componentInstanceId || !Array.isArray(child.plans)) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1013", parent, eventName, "projected child frame is invalid");
  }
  validateComponentFrameDefinition(child);
  const planIds = new Set(child.plans.map(plan => plan?.attachmentId));
  const expected = new Set(child.attachmentIds);
  if (planIds.size !== expected.size || [...expected].some(attachmentId => !planIds.has(attachmentId))) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1013", parent, eventName, "projected child plans do not match its attachment identities");
  }
}

function registerProjectedChildFrame(parent, definition, eventName) {
  if (componentFrames.has(definition.componentInstanceId)) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1003", parent, eventName, "duplicate child component frame " + definition.componentInstanceId);
  }
  const child = { ...definition, currentState: definition.initialState, lifecycle: "active" };
  destroyedComponentFrames.delete(child.componentInstanceId);
  componentFrames.set(child.componentInstanceId, child);
  traceComponentFrame("ChildFrameCreated", child, undefined, eventName);
  const unrelated = [...registeredAttachmentPlans.values()];
  registerAttachmentPlans({ schemaVersion: 1, projectId: "component-frame-child-create", plans: [...unrelated, ...child.plans] });
  for (const plan of child.plans) traceComponentFrame("AttachmentMounted", child, plan.attachmentId, eventName);
}

function replaceComponentFramePlans(frame, replacementPlans, eventName) {
  const expected = new Set(frame.attachmentIds);
  const received = new Set(replacementPlans.map(plan => plan?.attachmentId));
  if (expected.size !== received.size || [...expected].some(attachmentId => !received.has(attachmentId))) {
    throw componentFrameError("COPE-COMPONENT-STATE-BROWSER-1012", frame, eventName, "replacement plans do not match component attachment identities");
  }
  const unrelated = [...registeredAttachmentPlans.values()].filter(plan => !expected.has(plan.attachmentId));
  registerAttachmentPlans({ schemaVersion: 1, projectId: "component-frame-update", plans: [...unrelated, ...replacementPlans] });
  for (const plan of replacementPlans) traceComponentFrame("AttachmentUpdated", frame, plan.attachmentId, eventName);
}

function traceComponentFrame(kind, frame, attachmentId, eventName) {
  componentFrameTrace.push({ kind, componentInstanceId: frame.componentInstanceId, stateIdentity: frame.stateIdentity, attachmentId: attachmentId ?? null, eventName: eventName ?? null });
}

function componentFrameError(code, frame, eventName, message, cause) {
  const detail = cause instanceof Error ? ": " + cause.message : "";
  return new Error(code + " component=" + frame.componentInstanceId + " state=" + frame.stateIdentity + " event=" + eventName + " " + message + detail);
}

export function copyText(text, onSuccess, onFailure) {
  if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
    navigator.clipboard.writeText(text).then(onSuccess, () => copyTextWithDocument(text, onSuccess, onFailure));
    return;
  }

  copyTextWithDocument(text, onSuccess, onFailure);
}

function copyTextWithDocument(text, onSuccess, onFailure) {
  const element = document.createElement("textarea");
  element.value = text;
  element.setAttribute("readonly", "");
  element.style.position = "fixed";
  element.style.opacity = "0";
  document.body.appendChild(element);
  element.select();

  const copied = document.execCommand("copy");
  element.remove();
  if (copied) onSuccess();
  else onFailure();
}

export function getViewportWidth() {
  return window.innerWidth;
}

export function subscribeViewport(onChange) {
  window.addEventListener("resize", onChange, { passive: true });
  window.addEventListener("orientationchange", onChange, { passive: true });

  return () => {
    window.removeEventListener("resize", onChange);
    window.removeEventListener("orientationchange", onChange);
  };
}

const rendererAttachments = new Map();
const registeredAttachmentPlans = new Map();
const pendingAttachmentPlans = new Map();
const attachmentLifecycleCounts = new Map();
let attachmentHostObserver;
const rendererAdapters = new Map();

// Adapter authors register opaque renderer executors here. Copeland selects an
// adapter in its attachment artifact; the runtime never substitutes one.
export function registerRendererAdapter(adapterId, adapter) {
  if (typeof adapterId !== "string" || adapterId.length === 0) {
    throw new Error("COPE-RENDERER-0002 adapter ID must be a non-empty string");
  }
  if (adapter === null || typeof adapter !== "object"
    || typeof adapter.mount !== "function"
    || typeof adapter.update !== "function"
    || typeof adapter.unmount !== "function") {
    throw new Error("COPE-RENDERER-0003 adapter=" + adapterId + " executor must implement mount, update, and unmount");
  }
  if (rendererAdapters.has(adapterId)) {
    throw new Error("COPE-RENDERER-0013 adapter=" + adapterId + " duplicate adapter registration");
  }
  rendererAdapters.set(adapterId, adapter);
}

// This is deliberately a capability check rather than a way for application
// code to select adapters. Compiler-emitted plans remain authoritative.
export function hasRendererAdapter(adapterId) {
  return rendererAdapters.has(adapterId);
}

registerRendererAdapter("CustomElement", {
    mount(plan) {
      if (!isCustomElementTag(plan.tagName)) {
        throw rendererError("COPE-RENDERER-0008", plan, "invalid Custom Element tag");
      }

      const host = resolveRendererHost(plan);
      const element = document.createElement(plan.tagName);
      element.setAttribute("label", plan.payload);
      element.setAttribute("data-copeland-attachment", plan.attachmentId);
      host.appendChild(element);
      const frame = componentFrames.get(plan.componentInstanceId);
      if (frame !== undefined && typeof frame.rendererEventName === "string") {
        element.addEventListener("click", () => dispatchComponentEvent(frame.componentInstanceId, frame.rendererEventName));
      }
      return element;
    },
    update(plan, element) {
      const host = resolveRendererHost(plan);
      element.setAttribute("label", plan.payload);
      if (element.parentElement !== host) {
        host.appendChild(element);
      }
    },
    unmount(plan, element) {
      element.remove();
    }
});

export function scheduleRendererAttachment(callback) {
  // React may schedule a host replacement in its first animation frame.
  // Attach only after that frame commits so adapters never race a host that
  // Copeland has already selected.
  requestAnimationFrame(() => requestAnimationFrame(callback));
}

// registerAttachmentPlans is the versioned browser boundary. It accepts only
// compiler-emitted data; application code never selects hosts or adapters.
export function registerAttachmentPlans(artifact) {
  validateAttachmentArtifact(artifact);
  const plans = [...artifact.plans].sort(compareAttachmentPlans);
  const incomingIds = new Set(plans.map(plan => plan.attachmentId));
  const removed = [...registeredAttachmentPlans.values()]
    .filter(plan => !incomingIds.has(plan.attachmentId))
    .sort((left, right) => compareAttachmentPlans(right, left));
  for (const plan of removed) removeAttachmentPlan(plan);
  for (const plan of plans) {
    const existing = registeredAttachmentPlans.get(plan.attachmentId);
    if (existing !== undefined) {
      if (JSON.stringify(existing) !== JSON.stringify(plan)) {
        replaceAttachmentPlan(existing, plan);
      }
      continue;
    }
    registeredAttachmentPlans.set(plan.attachmentId, plan);
    if (plan.payload !== null && plan.payload !== undefined) {
      waitForAttachmentHost(plan);
    }
  }
}

function removeAttachmentPlan(plan) {
  pendingAttachmentPlans.get(plan.attachmentId)?.stop();
  if (rendererAttachments.has(plan.attachmentId)) {
    try { detachRenderer(plan.attachmentId, plan.componentInstanceId); }
    catch (error) { throw contextualRendererError("COPE-ATTACHMENT-PLAN-1009", plan, "removed plan teardown failed", error); }
  }
  registeredAttachmentPlans.delete(plan.attachmentId);
}

export function shutdownAttachmentPlans() {
  for (const pending of pendingAttachmentPlans.values()) pending.stop();
  pendingAttachmentPlans.clear();
  const mounted = [...rendererAttachments.values()]
    .sort((left, right) => right.plan.attachmentId.localeCompare(left.plan.attachmentId));
  for (const attachment of mounted) {
    try { detachRenderer(attachment.plan.attachmentId, attachment.plan.componentInstanceId); }
    catch (error) { console.warn(contextualRendererError("COPE-ATTACHMENT-PLAN-1010", attachment.plan, "shutdown cleanup failed", error)); }
  }
  registeredAttachmentPlans.clear();
  attachmentHostObserver?.disconnect();
  attachmentHostObserver = undefined;
}

// Runtime-only inspection intentionally excludes DOM nodes and adapter roots.
export function inspectAttachmentRuntime(attachmentId) {
  const counts = attachmentLifecycleCounts.get(attachmentId) ?? { mounts: 0, updates: 0, unmounts: 0 };
  return { ...counts, mounted: rendererAttachments.has(attachmentId), pending: pendingAttachmentPlans.has(attachmentId) };
}

function validateAttachmentArtifact(artifact) {
  if (artifact === null || typeof artifact !== "object") throw new Error("COPE-ATTACHMENT-PLAN-1004 attachment artifact must be an object");
  if (artifact.schemaVersion !== 1) throw new Error("COPE-ATTACHMENT-PLAN-1001 unsupported attachment plan schema version " + artifact.schemaVersion);
  if (typeof artifact.projectId !== "string" || !Array.isArray(artifact.plans)) throw new Error("COPE-ATTACHMENT-PLAN-1004 attachment artifact is missing projectId or plans");
  const ids = new Set();
  for (const plan of artifact.plans) {
    if (plan === null || typeof plan !== "object" || typeof plan.attachmentId !== "string" || typeof plan.componentInstanceId !== "string" || typeof plan.hostBoxId !== "string" || typeof plan.hostSelector !== "string" || typeof plan.adapterId !== "string" || plan.lifecycle?.mount !== true || plan.lifecycle?.update !== true || plan.lifecycle?.unmount !== true) {
      throw new Error("COPE-ATTACHMENT-PLAN-1004 attachment plan has a missing required field");
    }
    if (ids.has(plan.attachmentId)) throw new Error("COPE-ATTACHMENT-PLAN-1002 duplicate attachment ID " + plan.attachmentId);
    ids.add(plan.attachmentId);
  }
}

function compareAttachmentPlans(left, right) {
  if (left.parentComponentInstanceId === right.componentInstanceId) return 1;
  if (right.parentComponentInstanceId === left.componentInstanceId) return -1;
  return left.attachmentId.localeCompare(right.attachmentId);
}

function waitForAttachmentHost(plan) {
  if (pendingAttachmentPlans.has(plan.attachmentId) || rendererAttachments.has(plan.attachmentId)) return;
  let stopped = false;
  let timeout;
  let observer;
  const stop = () => {
    if (stopped) return;
    stopped = true;
    observer.disconnect();
    clearTimeout(timeout);
    pendingAttachmentPlans.delete(plan.attachmentId);
  };
  const tryMount = () => {
    if (stopped || registeredAttachmentPlans.get(plan.attachmentId) !== plan || !document.querySelector(plan.hostSelector)) return;
    stop();
    try { mountEmittedAttachment(plan); }
    catch (error) { console.error(contextualRendererError("COPE-ATTACHMENT-PLAN-1008", plan, "automatic mount failed", error)); }
  };
  observer = new MutationObserver(tryMount);
  pendingAttachmentPlans.set(plan.attachmentId, { stop });
  observer.observe(document.documentElement, { childList: true, subtree: true });
  scheduleRendererAttachment(tryMount);
  timeout = setTimeout(() => {
    if (!stopped) {
      stop();
      console.error(rendererError("COPE-ATTACHMENT-PLAN-1007", plan, "semantic host never appeared"));
    }
  }, 5000);
}

function mountEmittedAttachment(plan) {
  const payload = plan.payload;
  if (plan.adapterId === "CustomElement") {
    if (payload === null || typeof payload !== "object" || typeof payload.tagName !== "string") {
      throw rendererError("COPE-ATTACHMENT-PLAN-1005", plan, "CustomElement payload is not wire-compatible");
    }
    attachRenderer(plan.attachmentId, plan.componentInstanceId, plan.hostSelector, plan.adapterId, payload.tagName, payload.label ?? "");
    return;
  }
  throw rendererError("COPE-ATTACHMENT-PLAN-1006", plan, "adapter payload executor is unavailable");
}

function ensureAttachmentHostObserver() {
  if (attachmentHostObserver !== undefined) return;
  attachmentHostObserver = new MutationObserver(() => {
    for (const attachment of [...rendererAttachments.values()]) {
      if (attachment.host.isConnected
        && attachment.host.matches(attachment.plan.hostSelector)
        && attachment.host.contains(attachment.root)) continue;
      recoverDisconnectedAttachment(attachment);
    }
  });
  attachmentHostObserver.observe(document.documentElement, { childList: true, subtree: true });
}

function recoverDisconnectedAttachment(attachment) {
  const registered = registeredAttachmentPlans.get(attachment.plan.attachmentId);
  if (registered === undefined) {
    console.error(rendererError("COPE-ATTACHMENT-PLAN-1011", attachment.plan, "stale ownership record survived plan removal"));
    return;
  }
  try {
    attachment.adapter.unmount(attachment.plan, attachment.root);
    rendererAttachments.delete(attachment.plan.attachmentId);
    const counts = attachmentLifecycleCounts.get(attachment.plan.attachmentId);
    if (counts !== undefined) counts.unmounts += 1;
  } catch (error) {
    console.error(contextualRendererError("COPE-ATTACHMENT-PLAN-1012", attachment.plan, "adapter cleanup failed during semantic host replacement", error));
    return;
  }
  waitForAttachmentHost(registered);
}

function replaceAttachmentPlan(previous, next) {
  if (previous.adapterId !== next.adapterId) {
    if (rendererAttachments.has(previous.attachmentId)) detachRenderer(previous.attachmentId, previous.componentInstanceId);
    registeredAttachmentPlans.set(next.attachmentId, next);
    if (next.payload !== null && next.payload !== undefined) waitForAttachmentHost(next);
    return;
  }
  registeredAttachmentPlans.set(next.attachmentId, next);
  if (rendererAttachments.has(next.attachmentId)) {
    const payload = next.payload;
    if (next.adapterId !== "CustomElement" || payload === null || typeof payload !== "object") throw rendererError("COPE-ATTACHMENT-PLAN-1005", next, "updated payload is not wire-compatible");
    updateRenderer(next.attachmentId, next.componentInstanceId, next.hostSelector, payload.label ?? "");
  } else if (next.payload !== null && next.payload !== undefined) {
    waitForAttachmentHost(next);
  }
}

export function attachRenderer(attachmentId, componentInstanceId, hostSelector, adapterId, tagName, payload) {
  const plan = rendererPlan(attachmentId, componentInstanceId, hostSelector, adapterId, tagName, payload);
  if (rendererAttachments.has(plan.attachmentId)) {
    throw rendererError("COPE-RENDERER-0004", plan, "duplicate mount");
  }

  const adapter = rendererAdapters.get(plan.adapterId);
  if (adapter === undefined) {
    throw rendererError("COPE-RENDERER-0001", plan, "adapter executor unavailable");
  }

  try {
    const host = resolveRendererHost(plan);
    const root = adapter.mount(plan);
    rendererAttachments.set(plan.attachmentId, { plan, adapter, root, host, emittedPlan: registeredAttachmentPlans.get(plan.attachmentId) });
    const counts = attachmentLifecycleCounts.get(plan.attachmentId) ?? { mounts: 0, updates: 0, unmounts: 0 };
    counts.mounts += 1;
    attachmentLifecycleCounts.set(plan.attachmentId, counts);
    ensureAttachmentHostObserver();
  } catch (error) {
    throw contextualRendererError("COPE-RENDERER-0010", plan, "mount failed", error);
  }
}

export function updateRenderer(attachmentId, componentInstanceId, hostSelector, payload) {
  const attachment = rendererAttachments.get(attachmentId);
  if (attachment === undefined) {
    throw new Error("COPE-RENDERER-0005 adapter=unknown attachment=" + attachmentId + " component=" + componentInstanceId + " update before mount or after unmount");
  }

  if (attachment.plan.componentInstanceId !== componentInstanceId) {
    throw rendererError("COPE-RENDERER-0009", attachment.plan, "component identity mismatch");
  }

  const plan = { ...attachment.plan, hostSelector, payload };
  try {
    attachment.adapter.update(plan, attachment.root);
    attachment.plan = plan;
    const counts = attachmentLifecycleCounts.get(attachmentId);
    if (counts !== undefined) counts.updates += 1;
  } catch (error) {
    throw contextualRendererError("COPE-RENDERER-0011", plan, "update failed", error);
  }
}

export function detachRenderer(attachmentId, componentInstanceId) {
  const attachment = rendererAttachments.get(attachmentId);
  if (attachment === undefined) {
    throw new Error("COPE-RENDERER-0005 adapter=unknown attachment=" + attachmentId + " component=" + componentInstanceId + " unmount before mount or after release");
  }

  if (attachment.plan.componentInstanceId !== componentInstanceId) {
    throw rendererError("COPE-RENDERER-0009", attachment.plan, "component identity mismatch");
  }

  try {
    attachment.adapter.unmount(attachment.plan, attachment.root);
    rendererAttachments.delete(attachmentId);
    const counts = attachmentLifecycleCounts.get(attachmentId);
    if (counts !== undefined) counts.unmounts += 1;
  } catch (error) {
    throw contextualRendererError("COPE-RENDERER-0006", attachment.plan, "cleanup failed", error);
  }
}

function rendererPlan(attachmentId, componentInstanceId, hostSelector, adapterId, tagName, payload) {
  return { attachmentId, componentInstanceId, hostSelector, adapterId, tagName, payload };
}

function resolveRendererHost(plan) {
  const host = document.querySelector(plan.hostSelector);
  if (!(host instanceof HTMLElement)) {
    throw rendererError("COPE-RENDERER-0007", plan, "Copeland host was not found");
  }
  return host;
}

function isCustomElementTag(tagName) {
  return typeof tagName === "string" && /^[a-z][a-z0-9]*-[a-z0-9-]+$/.test(tagName);
}

function rendererError(code, plan, message) {
  return new Error(code + " adapter=" + plan.adapterId + " attachment=" + plan.attachmentId + " component=" + plan.componentInstanceId + " host=" + plan.hostSelector + " " + message);
}

function contextualRendererError(code, plan, message, cause) {
  const detail = cause instanceof Error ? cause.message : String(cause);
  return rendererError(code, plan, message + ": " + detail);
}
