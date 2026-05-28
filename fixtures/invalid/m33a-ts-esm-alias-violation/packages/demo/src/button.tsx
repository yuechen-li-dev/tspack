import forbidden from "forbidden-dep";

export function renderButton(label: string) {
  return `${label}:${forbidden}`;
}
