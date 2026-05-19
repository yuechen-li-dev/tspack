#!/usr/bin/env node

import os from 'node:os';

function detectLinuxSession() {
  const hasDisplay = Boolean(process.env.DISPLAY || process.env.WAYLAND_DISPLAY);
  const hasDbus = Boolean(process.env.DBUS_SESSION_BUS_ADDRESS);
  const checks = [
    `display=${hasDisplay ? 'present' : 'missing'}`,
    `dbus_session=${hasDbus ? 'present' : 'missing'}`
  ];

  if (!hasDisplay) {
    return {
      candidate: 'webkitgtk',
      checks,
      blocker: 'DISPLAY/WAYLAND_DISPLAY is required for a WebKitGTK runtime session.',
      outcome: 'unavailable'
    };
  }

  return {
    candidate: 'webkitgtk',
    checks,
    blocker: 'Runtime scaffold is present, but a Linux WebKitGTK execution host is not yet wired in tspack inspect.',
    outcome: 'not-usable'
  };
}

function probePlatformWebView() {
  const platform = process.platform;

  if (platform === 'win32') {
    return {
      os: platform,
      candidate: 'webview2',
      checks: ['platform=windows'],
      blocker: 'WebView2 probe is not implemented yet in this Node probe.',
      outcome: 'not-usable'
    };
  }

  if (platform === 'darwin') {
    return {
      os: platform,
      candidate: 'wkwebview',
      checks: ['platform=macos'],
      blocker: 'WKWebView probe is not implemented yet in this Node probe.',
      outcome: 'not-usable'
    };
  }

  return {
    os: platform,
    ...detectLinuxSession()
  };
}

const result = probePlatformWebView();

console.log(JSON.stringify({
  os: result.os,
  osRelease: os.release(),
  backend: 'platform-webview',
  candidate: result.candidate,
  checks: result.checks,
  blocker: result.blocker,
  outcome: result.outcome
}, null, 2));
